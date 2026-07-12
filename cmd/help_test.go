package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/hhe48203-ctrl/canvas-cli/internal/api"
	"github.com/hhe48203-ctrl/canvas-cli/internal/canvas"
	"github.com/spf13/cobra"
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

func TestAPIInvokeHelpCoversGenericRequestModes(t *testing.T) {
	root := newRootCommand()
	invoke, _, err := root.Find([]string{"api", "invoke"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"path", "query", "form", "body", "content-type", "header", "all-pages", "include-headers", "confirm"} {
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

func TestOperationIDsUsedByHelpExist(t *testing.T) {
	for _, id := range []string{"courses.list", "context_modules_api.index", "wiki_pages_api.create"} {
		if _, ok := api.Find(id); !ok {
			t.Errorf("help references unknown operation %q", id)
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
	if err := emitHTTPResponse(context.Background(), client, first); err != nil {
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

func TestQuizArrayAnswerUsesRepeatedBracketFields(t *testing.T) {
	values := url.Values{}
	if err := addFormValue(values, "quiz_questions[0][answer]", []any{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	got := values["quiz_questions[0][answer][]"]
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("encoded answers = %#v", values)
	}
}
