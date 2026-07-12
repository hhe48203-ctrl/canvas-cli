// Command canvas-docs-gen converts Canvas' generated all_resources.html into
// OpenAPI metadata that can be consumed by tools/openapi-gen.
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type document struct {
	OpenAPI string                          `yaml:"openapi"`
	Info    info                            `yaml:"info"`
	Paths   map[string]map[string]operation `yaml:"paths"`
}

type info struct {
	Title   string `yaml:"title"`
	Version string `yaml:"version"`
}

type operation struct {
	OperationID string                `yaml:"operationId"`
	Summary     string                `yaml:"summary"`
	Description string                `yaml:"description,omitempty"`
	Tags        []string              `yaml:"tags,omitempty"`
	Parameters  []parameter           `yaml:"parameters,omitempty"`
	RequestBody *requestBody          `yaml:"requestBody,omitempty"`
	Security    []map[string][]string `yaml:"security,omitempty"`
	External    externalDocumentation `yaml:"externalDocs,omitempty"`
}

type parameter struct {
	Name        string `yaml:"name"`
	In          string `yaml:"in"`
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description,omitempty"`
	Schema      schema `yaml:"schema"`
}

type schema struct {
	Type    string   `yaml:"type,omitempty"`
	Enum    []string `yaml:"enum,omitempty"`
	Default string   `yaml:"default,omitempty"`
}

type requestBody struct {
	Required bool                 `yaml:"required,omitempty"`
	Content  map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema schema `yaml:"schema"`
}

type externalDocumentation struct {
	URL string `yaml:"url,omitempty"`
}

var (
	methodStart   = regexp.MustCompile(`<div class="method_details[^"]*">`)
	headingRE     = regexp.MustCompile(`(?s)<h2 class='api_method_name' name='([^']+)' data-subtopic='([^']*)'>.*?<a[^>]*>\s*(.*?)\s*</a>`)
	endpointRE    = regexp.MustCompile(`(?s)<h3 class='endpoint'>\s*([A-Z]+)\s+([^<]+?)\s*</h3>`)
	rowRE         = regexp.MustCompile(`(?s)<tr class="request-param[^"]*">(.*?)</tr>`)
	cellRE        = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	scopeRE       = regexp.MustCompile(`(?s)<code class="scope">\s*([^<]+?)\s*</code>`)
	pathParamRE   = regexp.MustCompile(`[:*]([A-Za-z0-9_]+)`)
	placeholderRE = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	enumRE        = regexp.MustCompile(`(?s)<code class=enum>(.*?)</code>`)
	defaultRE     = regexp.MustCompile(`(?i)defaults? to\s+['+]*([^.,;\s<]+)`)
)

func main() {
	htmlPath := flag.String("html", "", "Canvas all_resources.html file")
	outPath := flag.String("out", "", "OpenAPI YAML output file")
	sourceURL := flag.String("source-url", "https://documentation.instructure.com/doc/api/all_resources.html", "documentation URL used for operation links")
	flag.Parse()
	if *htmlPath == "" || *outPath == "" {
		fatal("-html and -out are required")
	}
	data, err := os.ReadFile(*htmlPath)
	if err != nil {
		fatal(err.Error())
	}

	doc := document{OpenAPI: "3.0.0", Info: info{Title: "Canvas LMS REST API", Version: "generated"}, Paths: map[string]map[string]operation{}}
	seenIDs := map[string]int{}
	blocks := splitBlocks(string(data))
	for _, block := range blocks {
		heading := headingRE.FindStringSubmatch(block)
		if heading == nil {
			continue
		}
		baseID := strings.TrimPrefix(heading[1], "method.")
		group := clean(heading[2])
		summary := clean(heading[3])
		params := parseParameters(block)
		scopes := scopeRE.FindAllStringSubmatch(block, -1)
		endpoints := endpointRE.FindAllStringSubmatch(block, -1)
		for endpointIndex, endpoint := range endpoints {
			method := strings.ToLower(clean(endpoint[1]))
			path := normalizePath(clean(endpoint[2]))
			id := baseID
			seenIDs[id]++
			if seenIDs[id] > 1 || endpointIndex > 0 {
				id = fmt.Sprintf("%s.%d", baseID, seenIDs[baseID])
			}
			opParams := parametersForEndpoint(params, path, method)
			op := operation{
				OperationID: id,
				Summary:     summary,
				Description: parseDescription(block),
				Tags:        []string{group},
				Parameters:  opParams,
				External:    externalDocumentation{URL: *sourceURL + "#" + heading[1]},
			}
			if method != "get" && method != "head" && hasBodyParameters(opParams) {
				op.RequestBody = &requestBody{Required: hasRequiredBodyParameter(opParams), Content: map[string]mediaType{"application/x-www-form-urlencoded": {Schema: schema{Type: "object"}}}}
			}
			if endpointIndex < len(scopes) {
				op.Security = []map[string][]string{{"canvas": {clean(scopes[endpointIndex][1])}}}
			}
			if doc.Paths[path] == nil {
				doc.Paths[path] = map[string]operation{}
			}
			doc.Paths[path][method] = op
		}
	}

	encoded, err := yaml.Marshal(doc)
	if err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(*outPath, encoded, 0o644); err != nil {
		fatal(err.Error())
	}
	fmt.Fprintf(os.Stderr, "canvas-docs-gen: generated %d paths from %d documentation blocks\n", len(doc.Paths), len(blocks))
}

func splitBlocks(data string) []string {
	indices := methodStart.FindAllStringIndex(data, -1)
	result := make([]string, 0, len(indices))
	for i, index := range indices {
		end := len(data)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		result = append(result, data[index[0]:end])
	}
	return result
}

type parsedParameter struct {
	name, typeName, description string
	required                    bool
	enum                        []string
	defaultValue                string
}

func parseParameters(block string) []parsedParameter {
	rows := rowRE.FindAllStringSubmatch(block, -1)
	result := make([]parsedParameter, 0, len(rows))
	for _, row := range rows {
		cells := cellRE.FindAllStringSubmatch(row[1], -1)
		if len(cells) < 4 {
			continue
		}
		name := clean(cells[0][1])
		typeName := normalizeType(clean(cells[2][1]))
		if strings.HasSuffix(name, "[]") {
			typeName = "array"
		}
		result = append(result, parsedParameter{
			name: name, required: strings.Contains(strings.ToLower(cells[1][1]), "required"),
			typeName: typeName, description: clean(cells[3][1]), enum: parseEnum(cells[3][1]), defaultValue: parseDefault(cells[3][1]),
		})
	}
	return result
}

func parametersForEndpoint(documented []parsedParameter, path, method string) []parameter {
	seen := map[string]bool{}
	var result []parameter
	for _, match := range placeholderRE.FindAllStringSubmatch(path, -1) {
		name := match[1]
		seen[name] = true
		result = append(result, parameter{Name: name, In: "path", Required: true, Schema: schema{Type: "string"}})
	}
	for _, p := range documented {
		if p.name == "" || seen[p.name] {
			continue
		}
		location := "query"
		if method != "get" && method != "head" && method != "delete" {
			location = "body"
		}
		result = append(result, parameter{Name: p.name, In: location, Required: p.required, Description: p.description, Schema: schema{Type: p.typeName, Enum: p.enum, Default: p.defaultValue}})
	}
	return result
}

func normalizePath(path string) string {
	return pathParamRE.ReplaceAllString(path, `{$1}`)
}

func normalizeType(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "array") || strings.HasPrefix(lower, "["):
		return "array"
	case strings.Contains(lower, "integer"):
		return "integer"
	case strings.Contains(lower, "boolean"):
		return "boolean"
	case strings.Contains(lower, "datetime") || strings.Contains(lower, "date"):
		return "string"
	case strings.Contains(lower, "hash") || strings.Contains(lower, "object"):
		return "object"
	default:
		return "string"
	}
}

func parseEnum(value string) []string {
	matches := enumRE.FindAllStringSubmatch(value, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, clean(match[1]))
	}
	return result
}

func parseDefault(value string) string {
	match := defaultRE.FindStringSubmatch(clean(value))
	if match == nil {
		return ""
	}
	return strings.Trim(match[1], `"'++`)
}

func parseDescription(block string) string {
	end := len(block)
	if index := strings.Index(block, "<h4>Request Parameters:</h4>"); index >= 0 {
		end = index
	}
	prefix := block[:end]
	start := strings.LastIndex(prefix, "</div>")
	if start < 0 {
		return ""
	}
	candidate := prefix[start+len("</div>"):]
	for _, marker := range []string{"<h4", "Returns a ", "Returns an ", "Returns "} {
		if index := strings.Index(candidate, marker); index >= 0 {
			candidate = candidate[:index]
		}
	}
	return clean(candidate)
}

func hasBodyParameters(params []parameter) bool {
	for _, p := range params {
		if p.In == "body" {
			return true
		}
	}
	return false
}

func hasRequiredBodyParameter(params []parameter) bool {
	for _, p := range params {
		if p.In == "body" && p.Required {
			return true
		}
	}
	return false
}

func clean(value string) string {
	value = tagRE.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "canvas-docs-gen:", message)
	os.Exit(1)
}

// Keep generated YAML stable if future parsing introduces unordered values.
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
