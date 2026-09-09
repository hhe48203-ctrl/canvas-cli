package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/hhe48203-ctrl/canvas-cli/internal/api"
	"github.com/hhe48203-ctrl/canvas-cli/internal/canvas"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func TestEveryUserCommandHasClearHelpAndExamples(t *testing.T) {
	root := newRootCommand()
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		for _, child := range command.Commands() {
			if child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			if strings.TrimSpace(child.Short) == "" {
				t.Errorf("%s has no short help", child.CommandPath())
			}
			if len(child.Commands()) == 0 && strings.TrimSpace(child.Example) == "" {
				t.Errorf("%s has no executable example", child.CommandPath())
			}
			walk(child)
		}
	}
	walk(root)
}

func TestFilesFlagsBelongToCorrectCommands(t *testing.T) {
	root := newRootCommand()
	download, _, err := root.Find([]string{"files", "download"})
	if err != nil {
		t.Fatal(err)
	}
	if download.Flags().Lookup("destination") == nil {
		t.Fatal("files download is missing --destination")
	}
	list, _, err := root.Find([]string{"files", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Flags().Lookup("destination") != nil {
		t.Fatal("files list unexpectedly has --destination")
	}
	if list.InheritedFlags().Lookup("output") == nil {
		t.Fatal("files list lost the global --output format flag")
	}
}

func TestEveryHighLevelListExposesQueryAndPaginationFlags(t *testing.T) {
	root := newRootCommand()
	commands := [][]string{
		{"courses", "list"},
		{"modules", "list"},
		{"assignments", "list"},
		{"files", "list"},
		{"quizzes", "list"},
		{"quizzes", "questions"},
	}
	for _, path := range commands {
		command, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, flag := range []string{"query", "all-pages", "include-headers"} {
			if command.Flags().Lookup(flag) == nil {
				t.Errorf("%s is missing --%s", command.CommandPath(), flag)
			}
		}
	}
}

func TestNoArgumentCommandsRejectExtraArguments(t *testing.T) {
	root := newRootCommand()
	for _, path := range [][]string{{"courses", "list"}, {"auth", "status"}, {"me"}} {
		command, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := command.Args(command, []string{"unexpected"}); err == nil {
			t.Errorf("%s accepted an extra argument", command.CommandPath())
		}
	}
}

func TestAPIInvokeHelpCoversGenericRequestModes(t *testing.T) {
	root := newRootCommand()
	invoke, _, err := root.Find([]string{"api", "invoke"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"path", "query", "form", "body", "content-type", "header", "all-pages", "include-headers", "confirm", "dry-run"} {
		if invoke.Flags().Lookup(flag) == nil {
			t.Errorf("api invoke is missing --%s", flag)
		}
	}
	help := invoke.Long + "\n" + invoke.Example
	for _, topic := range []string{"operation ID", "Canvas form", "stdin", "Link headers", "api/graphql"} {
		if !strings.Contains(help, topic) {
			t.Errorf("api invoke help does not explain %q", topic)
		}
	}
}

func TestAssignmentSubmitHasDryRun(t *testing.T) {
	root := newRootCommand()
	submit, _, err := root.Find([]string{"assignments", "submit"})
	if err != nil {
		t.Fatal(err)
	}
	if submit.Flags().Lookup("dry-run") == nil {
		t.Fatal("assignments submit is missing --dry-run")
	}
}

func TestRootVersionUsesUsageMetadataOffline(t *testing.T) {
	resetCommandGlobals(t)
	t.Setenv("CANVAS_BASE_URL", "")
	t.Setenv("CANVAS_API_TOKEN", "")
	root := newRootCommand()
	root.SetArgs([]string{"--version"})
	data, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := debug.ReadBuildInfo()
	if root.Version != usageVersion(info) || !strings.Contains(string(data), root.Version) {
		t.Fatalf("version = %q, output = %q", root.Version, data)
	}
	for _, readme := range []string{"README.md", "README.zh-CN.md"} {
		data, err := os.ReadFile(filepath.Join("..", readme))
		if err != nil || !strings.Contains(string(data), "canvas --version") {
			t.Fatalf("%s does not document --version: %v", readme, err)
		}
	}
}

func TestOperationIDsUsedByHelpExist(t *testing.T) {
	for _, id := range []string{"courses.list", "context_modules_api.index", "wiki_pages_api.create"} {
		if _, ok := api.Find(id); !ok {
			t.Errorf("help references unknown operation %q", id)
		}
	}
}

func TestCanvasLMSSkillIsInstallableAndLinked(t *testing.T) {
	skillPath := filepath.Join("..", "skills", "canvas-lms", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(data), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatalf("invalid skill frontmatter: %s", data)
	}
	var frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Name != "canvas-lms" || frontmatter.Description == "" {
		t.Fatalf("frontmatter = %#v", frontmatter)
	}
	for _, required := range []string{
		"canvas auth status", "CANVAS_API_TOKEN", "--dry-run", "--confirm",
		"canvas api search modules", "canvas api describe context_modules_api.index", "canvas api invoke context_modules_api.index --path course_id=COURSE_ID", "returned by a previous Canvas response",
	} {
		if !strings.Contains(string(data), required) {
			t.Errorf("skill is missing %q", required)
		}
	}
	if _, ok := api.Find("context_modules_api.index"); !ok {
		t.Fatal("skill references an unknown operation ID")
	}
	resetCommandGlobals(t)
	home := t.TempDir()
	configDir := filepath.Join(home, "config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("AppData", configDir)
	t.Setenv("CANVAS_BASE_URL", "")
	t.Setenv("CANVAS_API_TOKEN", "")
	root := newRootCommand()
	root.SetArgs([]string{"--json", "api", "invoke", "context_modules_api.index", "--path", "course_id=synthetic-course", "--dry-run"})
	output, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Target         string `json:"target"`
			TargetResolved bool   `json:"target_resolved"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Target != "/api/v1/courses/synthetic-course/modules" || envelope.Data.TargetResolved {
		t.Fatalf("skill dry run = %#v", envelope)
	}
	for _, readme := range []string{"README.md", "README.zh-CN.md"} {
		readmeData, err := os.ReadFile(filepath.Join("..", readme))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(readmeData), "skills/canvas-lms/SKILL.md") {
			t.Errorf("%s does not link the skill", readme)
		}
	}
}

func TestEmitHTTPResponseCollectsAllPages(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"id":2}]`))
			return
		}
		w.Header().Set("Link", `<`+server.URL+`/items?page=2&opaque=a,b>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()

	client := canvas.NewClient(server.URL, "token")
	first, err := client.Request(context.Background(), http.MethodGet, "/items", nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	oldAllPages, oldFormat, oldJSON, oldYAML, oldHeaders := allPages, format, jsonOutput, yamlOutput, includeHeaders
	allPages, format, jsonOutput, yamlOutput, includeHeaders = true, "json", false, false, false
	t.Cleanup(func() {
		os.Stdout = oldStdout
		allPages, format, jsonOutput, yamlOutput, includeHeaders = oldAllPages, oldFormat, oldJSON, oldYAML, oldHeaders
	})
	if err := emitHTTPResponse(context.Background(), client, first, nil); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(reader).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) != 2 || envelope.Data[1]["id"] != float64(2) {
		t.Fatalf("paginated output = %#v", envelope.Data)
	}
}

func TestPageAccumulatorCollectsCanvasCompoundDocuments(t *testing.T) {
	first := decodeJSON([]byte(`{
		"meta":{"primaryCollection":"comments"},
		"comments":[{"id":1}],
		"authors":[{"id":10,"name":"First"}]
	}`))
	combined, err := newPageAccumulator(first)
	if err != nil {
		t.Fatal(err)
	}
	second := decodeJSON([]byte(`{
		"meta":{"primaryCollection":"comments"},
		"comments":[{"id":2}],
		"authors":[{"id":10,"name":"First"},{"id":11,"name":"Second"}]
	}`))
	if err := combined.Append(second); err != nil {
		t.Fatal(err)
	}
	result := combined.Result().(map[string]any)
	if got := len(result["comments"].([]any)); got != 2 {
		t.Fatalf("comments = %#v", result["comments"])
	}
	if got := len(result["authors"].([]any)); got != 2 {
		t.Fatalf("authors = %#v", result["authors"])
	}
}

func TestDecodeJSONPreservesCanvasIntegerIDs(t *testing.T) {
	value, ok := decodeJSON([]byte(`{"id":9007199254740993}`)).(map[string]any)
	if !ok {
		t.Fatal("decoded response is not an object")
	}
	if got := value["id"]; got != json.Number("9007199254740993") {
		t.Fatalf("id = %#v; want an exact JSON number", got)
	}
}

func TestParseQuizAnswersPreservesNestedMatchingAnswer(t *testing.T) {
	payload, err := parseQuizAnswers([]byte(`{
		"attempt": 1,
		"validation_token": "token",
		"quiz_questions": [{
			"id": 101,
			"answer": [{"answer_id": 6, "match_id": 10}]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	questions := payload["quiz_questions"].([]any)
	answer := questions[0].(map[string]any)["answer"].([]any)
	pair := answer[0].(map[string]any)
	if pair["answer_id"] != json.Number("6") || pair["match_id"] != json.Number("10") {
		t.Fatalf("matching answer = %#v", pair)
	}
}

func TestParseQuizAnswersRequiresSessionCredentials(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"missing attempt", `{"validation_token":"token","quiz_questions":[{"id":1,"answer":"A"}]}`, "integer attempt"},
		{"invalid attempt", `{"attempt":0,"validation_token":"token","quiz_questions":[{"id":1,"answer":"A"}]}`, "positive integer"},
		{"missing token", `{"attempt":1,"quiz_questions":[{"id":1,"answer":"A"}]}`, "validation_token"},
		{"empty questions", `{"attempt":1,"validation_token":"token","quiz_questions":[]}`, "non-empty quiz_questions"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseQuizAnswers([]byte(test.input))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want text %q", err, test.want)
			}
		})
	}
}

func TestQuizCompletionSessionValues(t *testing.T) {
	values, err := quizSessionValues(2, "validation", "access")
	if err != nil {
		t.Fatal(err)
	}
	if values.Get("attempt") != "2" || values.Get("validation_token") != "validation" || values.Get("access_code") != "access" {
		t.Fatalf("completion form = %#v", values)
	}
}

func TestQuizCompletionRequiresOfficialSessionFields(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		token   string
		want    string
	}{
		{name: "attempt", attempt: 0, token: "validation", want: "--attempt"},
		{name: "validation token", attempt: 1, token: " ", want: "--validation-token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := quizSessionValues(test.attempt, test.token, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want text %q", err, test.want)
			}
		})
	}
}

func TestCollectUploadedFileIDsSupportsMultipleAssignmentFiles(t *testing.T) {
	uploaded := []string{}
	ids, err := collectUploadedFileIDs([]string{"part-1.pdf", "part-2.pdf"}, func(path string) (map[string]any, error) {
		uploaded = append(uploaded, path)
		return map[string]any{"id": len(uploaded)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(uploaded, ",") != "part-1.pdf,part-2.pdf" || strings.Join(ids, ",") != "1,2" {
		t.Fatalf("uploaded = %#v, ids = %#v", uploaded, ids)
	}
}
