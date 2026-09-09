package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPagesShowUsesEscapedCourseRoute(t *testing.T) {
	resetCommandGlobals(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/api/v1/courses/course%2F1/pages/course-overview" || r.URL.Query().Get("include[]") != "body" {
			t.Fatalf("request = %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"url":"course-overview","body":"<p>Read me</p>"}`))
	}))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")
	root := newRootCommand()
	root.SetArgs([]string{"--json", "pages", "show", "course/1", "course-overview", "--query", "include[]=body"})
	data, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Body string `json:"body"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || !envelope.OK || envelope.Data.Body != "<p>Read me</p>" {
		t.Fatalf("output = %q, envelope = %#v, err = %v", data, envelope, err)
	}
}

func TestPagesShowNotFoundUsesErrorEnvelope(t *testing.T) {
	isolateUsage(t)
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")
	stdout, stderr, exit := runUsage(t, false, "--json", "pages", "show", "123", "missing")
	if stdout != "" || exit != 1 {
		t.Fatalf("output = (%q, %q, %d)", stdout, stderr, exit)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			HTTPStatus int `json:"http_status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil || envelope.OK || envelope.Error.HTTPStatus != http.StatusNotFound {
		t.Fatalf("error = %q, envelope = %#v, err = %v", stderr, envelope, err)
	}
}
