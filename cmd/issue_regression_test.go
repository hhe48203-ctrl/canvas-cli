package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestKeyValueFlagsPreserveValuesAndOrder(t *testing.T) {
	pairs, err := parsePairs([]string{"include[]=one", "include[]=two", "empty=", "equals=a=b"}, "query")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(pairs["include[]"], ","); got != "one,two" || pairs.Get("empty") != "" || pairs.Get("equals") != "a=b" {
		t.Fatalf("pairs = %#v", pairs)
	}

	path, err := parseMap([]string{"course_id=1", "empty="}, "path")
	if err != nil || path["course_id"] != "1" || path["empty"] != "" {
		t.Fatalf("path = %#v, err = %v", path, err)
	}
	headers, err := parseHeaders([]string{"X-Request-ID=a=b"}, "header")
	if err != nil || headers.Get("X-Request-ID") != "a=b" {
		t.Fatalf("headers = %#v, err = %v", headers, err)
	}

	for _, flag := range []string{"path", "query", "form", "header"} {
		for _, value := range []string{"missing", "=empty"} {
			if _, _, err := parsePair(value, flag); err == nil || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("parsePair(%q, %q) error = %v", value, flag, err)
			}
		}
	}
}

func TestAPIInvokeDryRunPreviewsEncodedRequestWithoutHTTP(t *testing.T) {
	resetCommandGlobals(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "never-display-this-token")

	root := newRootCommand()
	root.SetArgs([]string{"--json", "api", "invoke", "POST", "/api/v1/courses/{course_id}/items?existing=one",
		"--path", "course_id=room/7", "--query", "include[]=one", "--query", "include[]=two",
		"--form", "tags[]=alpha", "--form", "tags[]=beta", "--header", "Authorization=override",
		"--header", "Cookie=session=private", "--header", "X-Request-ID=request-5", "--dry-run"})
	data, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun      bool                `json:"dry_run"`
			Method      string              `json:"method"`
			Target      string              `json:"target"`
			Query       map[string][]string `json:"query"`
			ContentType string              `json:"content_type"`
			Body        string              `json:"body"`
			Headers     map[string][]string `json:"headers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	preview := envelope.Data
	if !envelope.OK || !preview.DryRun || preview.Method != "POST" ||
		preview.Target != "/api/v1/courses/room%2F7/items?existing=one&include%5B%5D=one&include%5B%5D=two" ||
		preview.ContentType != "application/x-www-form-urlencoded" || preview.Body != "tags%5B%5D=alpha&tags%5B%5D=beta" {
		t.Fatalf("preview = %#v", preview)
	}
	if got := strings.Join(preview.Query["include[]"], ","); got != "one,two" || preview.Query["existing"][0] != "one" {
		t.Fatalf("query = %#v", preview.Query)
	}
	if preview.Headers["Authorization"][0] != "[REDACTED]" || preview.Headers["Cookie"][0] != "[REDACTED]" ||
		preview.Headers["X-Request-Id"][0] != "request-5" || preview.Headers["Content-Type"][0] != "application/x-www-form-urlencoded" {
		t.Fatalf("headers = %#v", preview.Headers)
	}
	if requests != 0 || strings.Contains(string(data), "never-display-this-token") {
		t.Fatalf("requests = %d, output = %s", requests, data)
	}
}

func TestAPIInvokeDryRunSupportsOperationIDsAndYAML(t *testing.T) {
	resetCommandGlobals(t)
	t.Setenv("CANVAS_BASE_URL", "")
	t.Setenv("CANVAS_API_TOKEN", "")
	root := newRootCommand()
	root.SetArgs([]string{"--yaml", "api", "invoke", "courses.show", "--path", "id=123", "--dry-run"})
	data, err := captureCommandOutput(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dry_run: true") || !strings.Contains(string(data), "target: /api/v1/courses/123") {
		t.Fatalf("YAML preview = %s", data)
	}
}

func TestAPIInvokeDryRunValidatesParametersWithoutHTTP(t *testing.T) {
	resetCommandGlobals(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")

	for _, test := range []struct {
		name, want string
		args       []string
	}{
		{"query", "--query", []string{"api", "invoke", "GET", "/api/v1/courses", "--query", "missing", "--dry-run"}},
		{"header", "invalid header", []string{"api", "invoke", "GET", "/api/v1/courses", "--header", "Bad Header=value", "--dry-run"}},
		{"header value", "invalid header value", []string{"api", "invoke", "GET", "/api/v1/courses", "--header", "X-Test=bad\nvalue", "--dry-run"}},
		{"path", "missing required path parameter", []string{"api", "invoke", "courses.show", "--dry-run"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newRootCommand()
			root.SetArgs(test.args)
			if _, err := captureCommandOutput(t, root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("requests = %d; want 0", requests)
	}
}

func TestAPIInvokePaginationForwardsHeadersOnlySameOrigin(t *testing.T) {
	resetCommandGlobals(t)
	discardCommandOutput(t)

	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Request-ID"); got != "" {
			t.Errorf("cross-origin header = %q", got)
		}
		_, _ = w.Write([]byte(`[{"id":4}]`))
	}))
	defer foreign.Close()

	var canvasServer *httptest.Server
	canvasServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Request-ID"); got != "request-5" {
			t.Errorf("%s header = %q", r.URL.Path, got)
		}
		switch r.URL.Path {
		case "/same-first":
			w.Header().Set("Link", "<"+canvasServer.URL+"/same-second>; rel=\"next\"")
			_, _ = w.Write([]byte(`[{"id":1}]`))
		case "/same-second":
			_, _ = w.Write([]byte(`[{"id":2}]`))
		case "/foreign-first":
			w.Header().Set("Link", "<"+foreign.URL+"/second>; rel=\"next\"")
			_, _ = w.Write([]byte(`[{"id":3}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer canvasServer.Close()
	t.Setenv("CANVAS_BASE_URL", canvasServer.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")

	for _, path := range []string{"/same-first", "/foreign-first"} {
		headerArgs, allPages = nil, false
		root := newRootCommand()
		root.SetArgs([]string{"api", "invoke", "GET", path, "--header", "X-Request-ID=request-5", "--all-pages"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAssignmentFilePreflightStopsBeforeFirstUpload(t *testing.T) {
	resetCommandGlobals(t)
	discardCommandOutput(t)

	valid := filepath.Join(t.TempDir(), "valid.pdf")
	if err := os.WriteFile(valid, []byte("valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing.pdf")
	directory := t.TempDir()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")

	for name, invalid := range map[string]string{"missing": missing, "directory": directory} {
		t.Run(name, func(t *testing.T) {
			assignmentFiles, confirm = nil, false
			requests = 0
			root := newRootCommand()
			root.SetArgs([]string{"assignments", "submit", "1", "2", "--file", valid, "--file", invalid, "--confirm"})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", invalid)) {
				t.Fatalf("error = %v; want failing file %q", err, invalid)
			}
			if requests != 0 {
				t.Fatalf("requests = %d; want 0", requests)
			}
		})
	}
}

func TestAssignmentFileSubmissionPreservesFileOrder(t *testing.T) {
	resetCommandGlobals(t)
	discardCommandOutput(t)

	directory := t.TempDir()
	first, second := filepath.Join(directory, "first.pdf"), filepath.Join(directory, "second.pdf")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var initialized, submitted []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/courses/1/assignments/2/submissions/self/files":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			name := r.Form.Get("name")
			initialized = append(initialized, name)
			_, _ = fmt.Fprintf(w, `{"upload_url":%q,"upload_params":{}}`, server.URL+"/upload/"+name)
		case "/upload/first.pdf":
			_, _ = w.Write([]byte(`{"id":11}`))
		case "/upload/second.pdf":
			_, _ = w.Write([]byte(`{"id":22}`))
		case "/api/v1/courses/1/assignments/2/submissions/self":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			submitted = append([]string(nil), r.Form["submission[file_ids][]"]...)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")

	root := newRootCommand()
	root.SetArgs([]string{"assignments", "submit", "1", "2", "--file", first, "--file", second, "--confirm"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(initialized, ","); got != "first.pdf,second.pdf" {
		t.Fatalf("upload initialization order = %q", got)
	}
	if got := strings.Join(submitted, ","); got != "11,22" {
		t.Fatalf("submitted file IDs = %q", got)
	}
}

func resetCommandGlobals(t *testing.T) {
	t.Helper()
	oldBaseURL, oldFormat, oldBodyFile, oldContentType := baseURL, format, bodyFile, contentTypeFlag
	oldJSON, oldYAML, oldConfirm, oldDryRun, oldPages, oldHeaders := jsonOutput, yamlOutput, confirm, dryRun, allPages, includeHeaders
	oldPath, oldQuery, oldForm, oldHeader := pathArgs, queryArgs, formArgs, headerArgs
	oldFiles, oldText, oldURL, oldComment := assignmentFiles, assignmentText, assignmentURL, comment
	t.Cleanup(func() {
		baseURL, format, bodyFile, contentTypeFlag = oldBaseURL, oldFormat, oldBodyFile, oldContentType
		jsonOutput, yamlOutput, confirm, dryRun, allPages, includeHeaders = oldJSON, oldYAML, oldConfirm, oldDryRun, oldPages, oldHeaders
		pathArgs, queryArgs, formArgs, headerArgs = oldPath, oldQuery, oldForm, oldHeader
		assignmentFiles, assignmentText, assignmentURL, comment = oldFiles, oldText, oldURL, oldComment
	})
	baseURL, format, bodyFile, contentTypeFlag = "", "", "", "application/json"
	jsonOutput, yamlOutput, confirm, dryRun, allPages, includeHeaders = false, false, false, false, false, false
	pathArgs, queryArgs, formArgs, headerArgs = nil, nil, nil, nil
	assignmentFiles, assignmentText, assignmentURL, comment = nil, "", "", ""
}

func discardCommandOutput(t *testing.T) {
	t.Helper()
	file, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = file.Close()
	})
}

func captureCommandOutput(t *testing.T, root *cobra.Command) ([]byte, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	err = root.Execute()
	_ = writer.Close()
	os.Stdout = oldStdout
	data, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return data, err
}
