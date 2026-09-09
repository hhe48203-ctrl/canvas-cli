package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
