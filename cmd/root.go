package cmd

import (
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
	if err := rootCmd.Execute(); err != nil {
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
	cfg, err := config.Resolve(baseURL)
	if err != nil {
		return nil, err
	}
	return canvas.NewClient(cfg.BaseURL, cfg.Token), nil
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
		items, ok := data.([]any)
		if !ok {
			return fmt.Errorf("--all-pages requires an endpoint that returns a JSON array")
		}
		seen := map[string]bool{}
		for next := canvas.NextLink(resp.Headers); next != ""; next = canvas.NextLink(resp.Headers) {
			if seen[next] {
				return fmt.Errorf("pagination loop detected at %s", next)
			}
			seen[next] = true
			var err error
			resp, err = c.Request(ctx, http.MethodGet, next, nil, nil, "")
			if err != nil {
				return fmt.Errorf("fetch page %d: %w", pages+1, err)
			}
			page, ok := decodeJSON(resp.Body).([]any)
			if !ok {
				return fmt.Errorf("page %d did not return a JSON array", pages+1)
			}
			items = append(items, page...)
			pages++
		}
		data = items
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
	if json.Unmarshal(data, &value) == nil {
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
		return fmt.Errorf("this is a write operation; repeat with --confirm")
	}
	return nil
}
