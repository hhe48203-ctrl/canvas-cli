package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/hhe48203-ctrl/canvas-cli/internal/canvas"
	"github.com/hhe48203-ctrl/canvas-cli/internal/config"
	"github.com/hhe48203-ctrl/canvas-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	baseURL         string
	format          string
	jsonOutput      bool
	yamlOutput      bool
	bodyFile        string
	queryArgs       []string
	headerArgs      []string
	contentTypeFlag string
	pathArgs        []string
	formArgs        []string
	confirm         bool
	allPages        bool
	includeHeaders  bool
	rootCmd         = newRootCommand()
)

func Execute() {
	if err := executeWithUsage(rootCmd); err != nil {
		mode := outputMode()
		if mode == "table" {
			fmt.Fprintln(os.Stderr, "canvas:", err)
		} else if printErr := output.PrintTo(os.Stderr, output.Failure(err), mode); printErr != nil {
			fmt.Fprintln(os.Stderr, "canvas:", err)
		}
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "canvas",
		Short:         "Canvas LMS command-line client for university courses",
		Long:          "Work with university Canvas LMS courses from a terminal or AI agent using human-friendly commands, structured output, and a discoverable generic REST API invoker.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Example: `  # Configure credentials, then verify them
  export CANVAS_BASE_URL=https://school.instructure.com
  export CANVAS_API_TOKEN=token
  canvas auth status

  # Discover and invoke any registered Canvas REST operation
  canvas api search modules
  canvas api describe context_modules_api.index
  canvas api invoke GET /api/v1/courses --query enrollment_type=student

  # Machine-readable output
  canvas courses list --all-pages --json`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			selected := 0
			if jsonOutput {
				selected++
			}
			if yamlOutput {
				selected++
			}
			if format != "" && format != "auto" {
				selected++
			}
			if selected > 1 {
				return fmt.Errorf("choose only one of --json, --yaml, or --output")
			}
			switch format {
			case "", "auto", "table", "json", "yaml":
				return nil
			default:
				return fmt.Errorf("invalid output format %q; expected auto, table, json, or yaml", format)
			}
		},
	}
	root.PersistentFlags().StringVar(&baseURL, "base-url", "", "Canvas instance URL (or CANVAS_BASE_URL)")
	root.PersistentFlags().StringVar(&format, "output", "", "Output format: auto, table, json, yaml")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output JSON")
	root.PersistentFlags().BoolVar(&yamlOutput, "yaml", false, "Output YAML")
	root.AddCommand(newAuthCommand(), newAPICommand(), newCoursesCommand(), newAssignmentsCommand(), newFilesCommand(), newQuizzesCommand(), newMeCommand())
	return root
}

func client() (*canvas.Client, error) {
	if activeUsage != nil {
		activeUsage.phase = "configuration"
	}
	cfg, err := config.Resolve(baseURL)
	if err != nil {
		return nil, err
	}
	c := canvas.NewClient(cfg.BaseURL, cfg.Token)
	if activeUsage != nil {
		activeUsage.phase = "execution"
		c.HTTPClient.Transport = usageTransport{c.HTTPClient.Transport, activeUsage}
	}
	return c, nil
}

func contextWithClient() (context.Context, *canvas.Client, error) {
	c, err := client()
	return context.Background(), c, err
}

func outputMode() string {
	if jsonOutput {
		return "json"
	}
	if yamlOutput {
		return "yaml"
	}
	if format != "" && format != "auto" {
		return format
	}
	if format == "auto" {
		return "json"
	}
	if stat, err := os.Stdout.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		return "table"
	}
	return "json"
}

func emit(data any) error {
	return output.Print(output.Success(data), outputMode())
}

func emitRaw(data any) error {
	return output.Print(data, outputMode())
}

func emitHTTPResponse(ctx context.Context, c *canvas.Client, resp canvas.Response) error {
	data := decodeJSON(resp.Body)
	pages := 1
	if allPages {
		combined, err := newPageAccumulator(data)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for next := canvas.NextLink(resp.Headers); next != ""; next = canvas.NextLink(resp.Headers) {
			if seen[next] {
				return fmt.Errorf("pagination loop detected at %s", next)
			}
			seen[next] = true
			resp, err = c.Request(ctx, http.MethodGet, next, nil, nil, "")
			if err != nil {
				return fmt.Errorf("fetch page %d: %w", pages+1, err)
			}
			if err := combined.Append(decodeJSON(resp.Body)); err != nil {
				return fmt.Errorf("page %d: %w", pages+1, err)
			}
			pages++
		}
		data = combined.Result()
	}
	if includeHeaders {
		return emit(map[string]any{
			"status_code": resp.StatusCode,
			"headers":     resp.Headers,
			"pages":       pages,
			"data":        data,
		})
	}
	return emit(data)
}

type pageAccumulator struct {
	items    []any
	compound map[string]any
	primary  string
}

func newPageAccumulator(data any) (*pageAccumulator, error) {
	if items, ok := data.([]any); ok {
		return &pageAccumulator{items: items}, nil
	}
	compound, primary, items, err := compoundPage(data)
	if err != nil {
		return nil, fmt.Errorf("--all-pages %w", err)
	}
	return &pageAccumulator{items: items, compound: compound, primary: primary}, nil
}

func (a *pageAccumulator) Append(data any) error {
	if a.compound == nil {
		items, ok := data.([]any)
		if !ok {
			return fmt.Errorf("did not return a JSON array")
		}
		a.items = append(a.items, items...)
		return nil
	}

	compound, primary, items, err := compoundPage(data)
	if err != nil {
		return err
	}
	if primary != a.primary {
		return fmt.Errorf("compound document changed primary collection from %q to %q", a.primary, primary)
	}
	a.items = append(a.items, items...)
	mergeSecondaryCollections(a.compound, compound, primary)
	return nil
}

func (a *pageAccumulator) Result() any {
	if a.compound == nil {
		return a.items
	}
	a.compound[a.primary] = a.items
	return a.compound
}

func compoundPage(data any) (map[string]any, string, []any, error) {
	document, ok := data.(map[string]any)
	if !ok {
		return nil, "", nil, fmt.Errorf("requires an endpoint that returns a JSON array or Canvas compound document")
	}
	meta, ok := document["meta"].(map[string]any)
	if !ok {
		return nil, "", nil, fmt.Errorf("requires a compound document with meta.primaryCollection")
	}
	primary, ok := meta["primaryCollection"].(string)
	if !ok || primary == "" {
		return nil, "", nil, fmt.Errorf("requires a compound document with meta.primaryCollection")
	}
	items, ok := document[primary].([]any)
	if !ok {
		return nil, "", nil, fmt.Errorf("compound document primary collection %q is not an array", primary)
	}
	return document, primary, items, nil
}

func mergeSecondaryCollections(destination, source map[string]any, primary string) {
	for name, value := range source {
		if name == primary {
			continue
		}
		incoming, ok := value.([]any)
		if !ok {
			continue
		}
		existing, _ := destination[name].([]any)
		seen := make(map[string]bool, len(existing))
		for _, item := range existing {
			seen[compoundItemKey(item)] = true
		}
		for _, item := range incoming {
			key := compoundItemKey(item)
			if !seen[key] {
				existing = append(existing, item)
				seen[key] = true
			}
		}
		destination[name] = existing
	}
}

func compoundItemKey(item any) string {
	if object, ok := item.(map[string]any); ok {
		if id, exists := object["id"]; exists {
			return "id:" + fmt.Sprint(id)
		}
	}
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Sprintf("%#v", item)
	}
	return string(data)
}

func parsePairs(values []string) url.Values {
	result := url.Values{}
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			result.Add(item, "")
			continue
		}
		result.Add(key, value)
	}
	return result
}

func parseMap(values []string) map[string]string {
	result := map[string]string{}
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func parseHeaders(values []string) http.Header {
	result := http.Header{}
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result.Add(key, value)
		}
	}
	return result
}

func decodeJSON(data []byte) any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&value) == nil {
		return value
	}
	return string(data)
}

func pathWithParams(path string, params map[string]string) string {
	for key, value := range params {
		path = strings.ReplaceAll(path, "{"+key+"}", url.PathEscape(value))
	}
	return path
}

func readBody() ([]byte, error) {
	if bodyFile == "" {
		return nil, nil
	}
	if bodyFile == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(bodyFile)
}

func requireConfirm() error {
	if !confirm {
		return errConfirmRequired
	}
	return nil
}
