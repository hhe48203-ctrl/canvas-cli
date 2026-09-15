package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModulesListUsesCourseRoute(t *testing.T) {
	resetCommandGlobals(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/courses/123/modules" || r.URL.Query().Get("search_term") != "week 1" || r.URL.Query().Get("include[]") != "items" {
			t.Fatalf("request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"id":1,"name":"Module"}]`))
	}))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")
	root := newRootCommand()
	root.SetArgs([]string{"--json", "modules", "list", "123", "--query", "search_term=week 1", "--query", "include[]=items"})
	data, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID float64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !envelope.OK || len(envelope.Data) != 1 || envelope.Data[0].ID != 1 {
		t.Fatalf("output = %q, envelope = %#v, err = %v", data, envelope, err)
	}
	command, _, err := newRootCommand().Find([]string{"modules", "list"})
	if err != nil || command.Args(command, []string{"123"}) != nil || command.Args(command, []string{"123", "extra"}) == nil {
		t.Fatalf("modules list arguments = %v", err)
	}
}

func TestModulesItemsListUsesRouteAndPagination(t *testing.T) {
	resetCommandGlobals(t)
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v1/courses/course%2F1/modules/module%2F2/items" {
			t.Fatalf("request = %s %s", r.Method, r.URL.EscapedPath())
		}
		switch r.URL.Query().Get("page") {
		case "":
			if r.URL.Query().Get("include[]") != "content_details" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			w.Header().Set("Link", fmt.Sprintf("<%s/api/v1/courses/course%%2F1/modules/module%%2F2/items?page=2>; rel=\"next\"", server.URL))
			_, _ = w.Write([]byte(`[{"id":1,"type":"Page","page_url":"overview","content_id":7,"locked":true,"completion_requirement":{"type":"must_view"}}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"id":2,"type":"ExternalUrl","completion_requirement":{"type":"must_mark_done"}}]`))
		default:
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
	}))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")
	root := newRootCommand()
	root.SetArgs([]string{"--json", "modules", "items", "list", "course/1", "module/2", "--query", "include[]=content_details", "--all-pages"})
	data, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool             `json:"ok"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !envelope.OK || len(envelope.Data) != 2 ||
		envelope.Data[0]["type"] != "Page" || envelope.Data[0]["page_url"] != "overview" || envelope.Data[0]["content_id"] != float64(7) ||
		envelope.Data[0]["locked"] != true || envelope.Data[1]["completion_requirement"].(map[string]any)["type"] != "must_mark_done" {
		t.Fatalf("output = %q, envelope = %#v, err = %v", data, envelope, err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d; want 2", requests)
	}
	command, _, err := newRootCommand().Find([]string{"modules", "items", "list"})
	if err != nil || command.Args(command, []string{"123", "456"}) != nil || command.Args(command, []string{"123"}) == nil {
		t.Fatalf("modules items list arguments = %v", err)
	}
	for _, flag := range []string{"query", "all-pages", "include-headers"} {
		if command.Flags().Lookup(flag) == nil {
			t.Errorf("modules items list is missing --%s", flag)
		}
	}
	modules, _, err := newRootCommand().Find([]string{"modules"})
	if err != nil || !strings.Contains(modules.Long, "include[]=items") || !strings.Contains(modules.Long, "modules items list") {
		t.Fatalf("modules help = %q, error = %v", modules.Long, err)
	}
}
