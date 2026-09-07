package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	oldJSON, oldYAML, oldConfirm, oldPages, oldHeaders := jsonOutput, yamlOutput, confirm, allPages, includeHeaders
	oldPath, oldQuery, oldForm, oldHeader := pathArgs, queryArgs, formArgs, headerArgs
	oldFiles, oldText, oldURL, oldComment := assignmentFiles, assignmentText, assignmentURL, comment
	t.Cleanup(func() {
		baseURL, format, bodyFile, contentTypeFlag = oldBaseURL, oldFormat, oldBodyFile, oldContentType
		jsonOutput, yamlOutput, confirm, allPages, includeHeaders = oldJSON, oldYAML, oldConfirm, oldPages, oldHeaders
		pathArgs, queryArgs, formArgs, headerArgs = oldPath, oldQuery, oldForm, oldHeader
		assignmentFiles, assignmentText, assignmentURL, comment = oldFiles, oldText, oldURL, oldComment
	})
	baseURL, format, bodyFile, contentTypeFlag = "", "", "", "application/json"
	jsonOutput, yamlOutput, confirm, allPages, includeHeaders = false, false, false, false, false
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
