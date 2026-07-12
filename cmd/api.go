package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/hhe48203-ctrl/canvas-cli/internal/api"
	"github.com/spf13/cobra"
)

func newAPICommand() *cobra.Command {
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "Discover, describe, and invoke Canvas REST API operations",
		Long: `Discover the bundled Canvas REST API catalog or invoke any endpoint directly.

Use 'api search' to find an operation, 'api describe' to inspect its required
parameters and body format, then 'api invoke' with an operation ID. Endpoints
not present in the catalog remain callable with an explicit METHOD and PATH.`,
		Example: `  canvas api search modules
  canvas api describe courses.list
  canvas api invoke courses.list --query enrollment_type=student
  canvas api invoke POST /api/v1/courses/123/pages --form 'wiki_page[title]=Overview' --confirm`,
	}
	apiCmd.AddCommand(newAPIListCommand(), newAPISearchCommand(), newAPIDescribeCommand(), newAPIInvokeCommand())
	return apiCmd
}

func newAPIListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every bundled Canvas REST API operation",
		Long:  "List the complete operation catalog bundled at build time. Use 'api search TERM' to narrow it and 'api describe ID' for parameter details.",
		Example: `  canvas api list
  canvas api list --json | jq '.data | length'
  canvas api search assignments`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return emitOperations(api.Operations)
		},
	}
}

func newAPISearchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search TERM",
		Short: "Search operations by ID, group, summary, method, or path",
		Args:  cobra.ExactArgs(1),
		Example: `  canvas api search modules
  canvas api search '/api/v1/accounts'
  canvas api search POST --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			term := strings.ToLower(args[0])
			var matches []api.Operation
			for _, op := range api.Operations {
				haystack := strings.ToLower(strings.Join([]string{op.ID, op.Group, op.Summary, op.Method, op.Path}, "\n"))
				if strings.Contains(haystack, term) {
					matches = append(matches, op)
				}
			}
			if len(matches) == 0 {
				return fmt.Errorf("no Canvas API operations match %q", args[0])
			}
			return emitOperations(matches)
		},
	}
}

func newAPIDescribeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "describe OPERATION_ID",
		Short: "Show parameters, request body, scopes, and responses for an operation",
		Args:  cobra.ExactArgs(1),
		Example: `  canvas api describe courses.list
	canvas api describe wiki_pages_api.create --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			op, ok := api.Find(args[0])
			if !ok {
				return fmt.Errorf("unknown operation %q; run 'canvas api search TERM' or invoke a raw METHOD PATH", args[0])
			}
			return emitOperation(op)
		},
	}
}

func newAPIInvokeCommand() *cobra.Command {
	invoke := &cobra.Command{
		Use:   "invoke OPERATION_ID | METHOD PATH",
		Short: "Invoke a registered operation or an arbitrary Canvas REST endpoint",
		Long: `Invoke an operation ID from 'api list', or provide an HTTP method and path.

Path parameters for registered operations use --path name=value. Query and form
flags are repeatable, including array names such as include[]. Use --body for a
raw JSON or other pre-encoded body; use --body - to read stdin. Canvas list
responses are paginated, so use --all-pages to follow opaque Link headers.`,
		Example: `  # Registered operation
  canvas api invoke courses.list --query enrollment_type=student --all-pages

  # Raw GET with repeated array query parameters
  canvas api invoke GET /api/v1/calendar_events \
    --query 'context_codes[]=course_123' --query 'context_codes[]=course_456'

  # Standard Canvas form request
  canvas api invoke POST /api/v1/courses/123/pages \
    --form 'wiki_page[title]=Overview' \
    --form 'wiki_page[body]=<p>Hello</p>' --confirm

  # Raw JSON from stdin
  printf '%s' '{"query":"{ course(id: \"123\") { name } }"}' | \
    canvas api invoke POST /api/graphql --body - --content-type application/json --confirm`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method, path := "", ""
			var selected *api.Operation
			if op, ok := api.Find(args[0]); ok {
				if len(args) != 1 {
					return fmt.Errorf("operation ID form accepts one argument; remove %q", args[1])
				}
				method, path = op.Method, op.Path
				selected = &op
				provided := parseMap(pathArgs)
				for _, parameter := range op.ParametersIn("path") {
					if _, ok := provided[parameter.Name]; !ok {
						return fmt.Errorf("missing required path parameter %q; use --path %s=VALUE", parameter.Name, parameter.Name)
					}
				}
				path = pathWithParams(path, provided)
			} else {
				if len(args) != 2 {
					return fmt.Errorf("unknown operation %q; expected a known OPERATION_ID or METHOD PATH", args[0])
				}
				method, path = strings.ToUpper(args[0]), pathWithParams(args[1], parseMap(pathArgs))
			}
			if strings.Contains(path, "{") {
				return fmt.Errorf("unresolved path parameter in %q; supply each value with --path name=value", path)
			}
			if !supportedHTTPMethod(method) {
				return fmt.Errorf("unsupported HTTP method %q; expected GET, HEAD, OPTIONS, POST, PUT, PATCH, or DELETE", method)
			}
			if method != http.MethodGet && allPages {
				return fmt.Errorf("--all-pages is only valid for GET requests")
			}
			if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions && !confirm {
				return fmt.Errorf("this is a write operation; repeat with --confirm after reviewing the request")
			}

			query := parsePairs(queryArgs)
			form := parsePairs(formArgs)
			if selected != nil {
				for _, parameter := range selected.ParametersIn("query") {
					if parameter.Required && !query.Has(parameter.Name) {
						return fmt.Errorf("missing required query parameter %q; use --query %q", parameter.Name, parameter.Name+"=VALUE")
					}
				}
				if bodyFile == "" {
					for _, parameter := range selected.ParametersIn("body") {
						if parameter.Required && !form.Has(parameter.Name) {
							return fmt.Errorf("missing required form field %q; use --form %q or provide --body", parameter.Name, parameter.Name+"=VALUE")
						}
					}
					if selected.RequestBody != nil && selected.RequestBody.Required && len(form) == 0 {
						return fmt.Errorf("operation %s requires a request body; use --form or --body", selected.ID)
					}
				}
			}

			body, err := readBody()
			if err != nil {
				return err
			}
			contentType := ""
			if len(formArgs) > 0 {
				body = []byte(form.Encode())
				contentType = "application/x-www-form-urlencoded"
			} else if bodyFile != "" {
				contentType = contentTypeFlag
			}
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			resp, err := c.RequestWithHeaders(ctx, method, path, query, bytes.NewReader(body), contentType, parseHeaders(headerArgs))
			if err != nil {
				return err
			}
			return emitHTTPResponse(ctx, c, resp)
		},
	}
	invoke.Flags().StringArrayVar(&pathArgs, "path", nil, "Path parameter name=value; repeat for every placeholder")
	invoke.Flags().StringArrayVar(&queryArgs, "query", nil, "Query parameter name=value; repeat the flag for array values")
	invoke.Flags().StringArrayVar(&formArgs, "form", nil, "Form field name=value using Canvas nested names; repeat for arrays")
	invoke.Flags().StringArrayVar(&headerArgs, "header", nil, "Additional HTTP header name=value; repeat for multiple values")
	invoke.Flags().StringVar(&bodyFile, "body", "", "Raw request body file, or - to read stdin")
	invoke.Flags().StringVar(&contentTypeFlag, "content-type", "application/json", "Content-Type used with --body")
	invoke.Flags().BoolVar(&allPages, "all-pages", false, "Follow Canvas Link headers and combine all JSON-array pages")
	invoke.Flags().BoolVar(&includeHeaders, "include-headers", false, "Include HTTP status, headers, page count, and data")
	invoke.Flags().BoolVar(&confirm, "confirm", false, "Confirm POST, PUT, PATCH, or DELETE after reviewing the request")
	invoke.MarkFlagsMutuallyExclusive("body", "form")
	return invoke
}

func emitOperations(operations []api.Operation) error {
	if outputMode() != "table" {
		return emit(operations)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "OPERATION ID\tMETHOD\tPATH\tSUMMARY")
	for _, op := range operations {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", op.ID, op.Method, op.Path, op.Summary)
	}
	return w.Flush()
}

func emitOperation(op api.Operation) error {
	if outputMode() != "table" {
		return emit(op)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "Operation:\t%s\nMethod:\t%s\nPath:\t%s\nGroup:\t%s\nSummary:\t%s\n", op.ID, op.Method, op.Path, op.Group, op.Summary)
	if op.Description != "" {
		fmt.Fprintf(w, "Description:\t%s\n", op.Description)
	}
	if op.DocsURL != "" {
		fmt.Fprintf(w, "Documentation:\t%s\n", op.DocsURL)
	}
	if len(op.Scopes) > 0 {
		fmt.Fprintf(w, "Scopes:\t%s\n", strings.Join(op.Scopes, ", "))
	}
	params := append([]api.Parameter(nil), op.Parameters...)
	for _, location := range []string{"path", "query"} {
		params = append(params, op.ParametersIn(location)...)
	}
	params = deduplicateParameters(params)
	if len(params) > 0 {
		fmt.Fprintln(w, "\nPARAMETER\tIN\tREQUIRED\tTYPE\tDEFAULT / ALLOWED VALUES\tDESCRIPTION")
		for _, p := range params {
			allowed := p.Default
			if len(p.Enum) > 0 {
				allowed = strings.Join(p.Enum, ", ")
			}
			fmt.Fprintf(w, "%s\t%s\t%t\t%s\t%s\t%s\n", p.Name, p.In, p.Required, p.Type, allowed, p.Description)
		}
	}
	if op.RequestBody != nil {
		fmt.Fprintf(w, "\nRequest body:\trequired=%t; content types: %s\n", op.RequestBody.Required, strings.Join(op.RequestBody.ContentTypes, ", "))
	}
	if len(op.Responses) > 0 {
		fmt.Fprintln(w, "\nSTATUS\tCONTENT TYPES\tDESCRIPTION")
		for _, response := range op.Responses {
			fmt.Fprintf(w, "%s\t%s\t%s\n", response.StatusCode, strings.Join(response.ContentTypes, ", "), response.Description)
		}
	}
	fmt.Fprintf(w, "\nExample:\tcanvas api invoke %s", op.ID)
	for _, p := range op.ParametersIn("path") {
		fmt.Fprintf(w, " --path %s=VALUE", p.Name)
	}
	for _, p := range op.ParametersIn("query") {
		if p.Required {
			fmt.Fprintf(w, " --query %q", p.Name+"=VALUE")
		}
	}
	for _, p := range op.ParametersIn("body") {
		if p.Required {
			fmt.Fprintf(w, " --form %q", p.Name+"=VALUE")
		}
	}
	if op.Method != http.MethodGet && op.Method != http.MethodHead && op.Method != http.MethodOptions {
		fmt.Fprint(w, " --confirm")
	}
	fmt.Fprintln(w)
	return w.Flush()
}

func deduplicateParameters(values []api.Parameter) []api.Parameter {
	seen := map[string]bool{}
	result := make([]api.Parameter, 0, len(values))
	for _, p := range values {
		key := p.In + "\x00" + p.Name
		if !seen[key] {
			seen[key] = true
			result = append(result, p)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		order := map[string]int{"path": 0, "query": 1, "body": 2, "header": 3}
		if order[result[i].In] == order[result[j].In] {
			return result[i].Name < result[j].Name
		}
		return order[result[i].In] < order[result[j].In]
	})
	return result
}

func supportedHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
