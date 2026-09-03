package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Exercise the real entry point, including os.Exit, without touching user logs.
func TestUsageProcess(t *testing.T) {
	if os.Getenv("CANVAS_USAGE_TEST_HELPER") != "1" {
		return
	}
	i := slices.Index(os.Args, "--")
	rootCmd = newRootCommand()
	rootCmd.SetArgs(os.Args[i+1:])
	Execute()
	os.Exit(0)
}

func isolateUsage(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("AppData", filepath.Join(home, "config"))
	t.Setenv("CANVAS_USAGE_LOG", "")
	t.Setenv("CANVAS_BASE_URL", "")
	t.Setenv("CANVAS_API_TOKEN", "")
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cache, "canvas-cli", "logs")
}

func runUsage(t *testing.T, enabled bool, args ...string) (string, string, int) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestUsageProcess$", "--"}, args...)...)
	command.Env = append(os.Environ(), "CANVAS_USAGE_TEST_HELPER=1")
	if !enabled {
		command.Env = append(command.Env, "CANVAS_USAGE_LOG=0")
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), command.ProcessState.ExitCode()
}

func readUsage(t *testing.T, dir string) []usageEvent {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []usageEvent
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			t.Fatalf("log is not newline terminated: %s", path)
		}
		for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
			var event usageEvent
			decoder := json.NewDecoder(bytes.NewReader(line))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&event); err != nil {
				t.Fatalf("invalid log line: %s: %v", line, err)
			}
			events = append(events, event)
		}
	}
	return events
}

func TestUsageCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("shape") == "object" {
			fmt.Fprint(w, `{"name":"SENSITIVE-response-body"}`)
			return
		}
		if r.URL.Path == "/denied" || r.URL.Query().Get("page") == "denied" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"SENSITIVE-response-body"}`)
			return
		}
		if page := r.URL.Query().Get("next"); page != "" {
			w.Header().Set("Link", fmt.Sprintf("<http://%s/api/v1/courses?page=%s>; rel=\"next\"", r.Host, page))
		}
		fmt.Fprint(w, `[{"id":1,"name":"SENSITIVE-response-body"}]`)
	}))
	defer server.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	untrustedTLS := httptest.NewTLSServer(http.NotFoundHandler())
	untrustedTLS.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer untrustedTLS.Close()

	tests := []struct {
		name, command, kind, errorKind, operation string
		args                                      []string
		status                                    int
	}{
		{"describe", "canvas api describe", "command", "", "courses.list", []string{"api", "describe", "courses.list", "--json"}, 0},
		{"invoke", "canvas api invoke", "command", "", "courses.list", []string{"api", "invoke", "courses.list", "--query", "q=SENSITIVE-query", "--header", "X-Private=SENSITIVE-header"}, 200},
		{"raw URL", "canvas api invoke", "command", "", "", []string{"api", "invoke", "GET", server.URL + "/SENSITIVE-path?key=SENSITIVE-query"}, 200},
		{"pagination", "canvas courses list", "command", "", "", []string{"courses", "list", "--all-pages", "--query", "next=2"}, 200},
		{"pagination failure", "canvas courses list", "command", "http", "", []string{"courses", "list", "--all-pages", "--query", "next=denied"}, 403},
		{"execution failure", "canvas courses list", "command", "execution", "", []string{"courses", "list", "--all-pages", "--query", "shape=object"}, 200},
		{"HTTP failure", "canvas api invoke", "command", "http", "", []string{"api", "invoke", "GET", "/denied"}, 403},
		{"network failure", "canvas me", "command", "network", "", []string{"me", "--base-url", closed.URL}, 0},
		{"TLS failure", "canvas me", "command", "network", "", []string{"me", "--base-url", untrustedTLS.URL}, 0},
		{"invalid URL escape", "canvas api invoke", "command", "arguments", "", []string{"api", "invoke", "GET", "/SENSITIVE%ZZ"}, 0},
		{"invalid header name", "canvas api invoke", "command", "arguments", "", []string{"api", "invoke", "GET", "/valid", "--header", "Bad Header=SENSITIVE-value"}, 0},
		{"invalid header value", "canvas api invoke", "command", "arguments", "", []string{"api", "invoke", "GET", "/valid", "--header", "X-Test=SENSITIVE\nvalue"}, 0},
		{"invalid config", "canvas me", "command", "configuration", "", []string{"me", "--base-url", "SENSITIVE-invalid-url"}, 0},
		{"invalid config escape", "canvas me", "command", "configuration", "", []string{"me", "--base-url", "http://SENSITIVE%ZZ"}, 0},
		{"missing token", "canvas me", "command", "configuration", "", []string{"me"}, 0},
		{"invalid saved URL", "canvas auth set-url", "command", "configuration", "", []string{"auth", "set-url", "SENSITIVE-invalid-url"}, 0},
		{"missing args", "canvas courses show", "command", "arguments", "", []string{"courses", "show"}, 0},
		{"extra args", "canvas me", "command", "arguments", "", []string{"me", "SENSITIVE-extra"}, 0},
		{"unknown command", "canvas", "command", "arguments", "", []string{"SENSITIVE-unknown-command"}, 0},
		{"unknown flag", "canvas me", "command", "arguments", "", []string{"me", "--SENSITIVE-unknown-flag"}, 0},
		{"unknown operation", "canvas api invoke", "command", "arguments", "", []string{"api", "invoke", "SENSITIVE-unknown-operation"}, 0},
		{"path validation", "canvas api invoke", "command", "arguments", "courses.show", []string{"api", "invoke", "courses.show"}, 0},
		{"output conflict", "canvas me", "command", "arguments", "", []string{"me", "--json", "--yaml"}, 0},
		{"flag group", "canvas api invoke", "command", "arguments", "courses.list", []string{"api", "invoke", "courses.list", "--body", "SENSITIVE-file", "--form", "key=SENSITIVE-form"}, 0},
		{"confirmation", "canvas assignments submit", "command", "confirmation_required", "", []string{"assignments", "submit", "SENSITIVE-course", "SENSITIVE-assignment", "--text", "SENSITIVE-answer"}, 0},
		{"API confirmation", "canvas api invoke", "command", "confirmation_required", "", []string{"api", "invoke", "POST", "/SENSITIVE-path", "--form", "key=SENSITIVE-form"}, 0},
		{"quiz confirmation", "canvas quizzes complete", "command", "confirmation_required", "", []string{"quizzes", "complete", "1", "2", "3", "--attempt", "1", "--validation-token", "SENSITIVE-validation", "--access-code", "SENSITIVE-access"}, 0},
		{"body file failure", "canvas api invoke", "command", "io", "courses.list", []string{"api", "invoke", "courses.list", "--body", "/SENSITIVE-missing-file"}, 0},
		{"help flag", "canvas me", "help", "", "", []string{"me", "--help"}, 0},
		{"explicit help", "canvas help", "help", "", "", []string{"help", "courses", "list"}, 0},
		{"implicit help", "canvas", "help", "", "", nil, 0},
		{"group help", "canvas courses", "help", "", "", []string{"courses"}, 0},
		{"help as value", "canvas api search", "command", "arguments", "", []string{"api", "search", "--", "--help"}, 0},
		{"completion script", "canvas completion bash", "completion", "", "", []string{"completion", "bash"}, 0},
		{"completion request", "canvas __complete", "completion", "", "", []string{"__complete", "courses", ""}, 0},
		{"completion alias", "canvas __complete", "completion", "", "", []string{"__completeNoDesc", "courses", ""}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := isolateUsage(t)
			t.Setenv("CANVAS_BASE_URL", server.URL)
			t.Setenv("CANVAS_API_TOKEN", "SENSITIVE-api-token")
			if test.name == "missing token" {
				t.Setenv("CANVAS_API_TOKEN", "")
			}
			wantOut, wantErr, wantExit := runUsage(t, false, test.args...)
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("disabled logging created a directory: %v", err)
			}
			out, stderr, exit := runUsage(t, true, test.args...)
			if out != wantOut || stderr != wantErr || exit != wantExit {
				t.Fatalf("logging changed output/exit: (%q, %q, %d), want (%q, %q, %d)", out, stderr, exit, wantOut, wantErr, wantExit)
			}
			events := readUsage(t, dir)
			if len(events) != 1 {
				t.Fatalf("got %d events, want one", len(events))
			}
			event := events[0]
			if event.Command != test.command || event.Kind != test.kind || event.ErrorKind != test.errorKind ||
				event.OperationID != test.operation || event.HTTPStatus != test.status || event.ExitCode != exit {
				t.Fatalf("unexpected event: %+v", event)
			}
			if event.Time.IsZero() || event.Version == "" || event.DurationMS < 0 || (test.errorKind != "" && exit != 1) {
				t.Fatalf("missing/invalid metadata: %+v", event)
			}
			data, _ := json.Marshal(event)
			if strings.Contains(string(data), "SENSITIVE") || strings.Contains(string(data), server.URL) {
				t.Fatalf("sensitive value in log: %s", data)
			}
		})
	}
}

func TestUsageFileTransfers(t *testing.T) {
	dir := isolateUsage(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/123/download":
			http.Redirect(w, r, "/download", http.StatusFound)
		case "/download":
			fmt.Fprint(w, "SENSITIVE-file-content")
		case "/api/v1/courses/123/files":
			fmt.Fprintf(w, `{"upload_url":"http://%s/upload","upload_params":{"key":"SENSITIVE-upload-key"}}`, r.Host)
		case "/upload":
			if err := r.ParseMultipartForm(1024); err != nil {
				t.Error(err)
				http.Error(w, "invalid upload", 400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			w.Header().Set("Location", fmt.Sprintf("http://%s/uploaded", r.Host))
			w.WriteHeader(http.StatusCreated)
		case "/uploaded":
			fmt.Fprint(w, `{"id":42}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "SENSITIVE-token")
	path := filepath.Join(t.TempDir(), "SENSITIVE-file.txt")
	for _, args := range [][]string{
		{"files", "download", "123", "--destination", path},
		{"files", "upload", "123", path, "--confirm"},
	} {
		wantOut, wantErr, wantExit := runUsage(t, false, args...)
		out, stderr, exit := runUsage(t, true, args...)
		if exit != 0 || out != wantOut || stderr != wantErr || exit != wantExit {
			t.Fatalf("transfer changed or failed: %q, %q, %d", out, stderr, exit)
		}
	}
	events := readUsage(t, dir)
	if len(events) != 2 {
		t.Fatalf("got %d records for two file commands", len(events))
	}
	for _, event := range events {
		data, _ := json.Marshal(event)
		if event.HTTPStatus != 200 || event.ExitCode != 0 || strings.Contains(string(data), "SENSITIVE") {
			t.Fatalf("incorrect transfer summary: %s", data)
		}
	}
}

func TestUsageStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"2026-08-27.jsonl", "2026-08-28.jsonl", "2026-09-03.jsonl", "keep.txt", "2026-99-99.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	event := usageEvent{Time: today, Version: "devel", Kind: "command", Command: "canvas me"}
	if err := appendUsage(dir, event); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-08-27.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired log was not removed: %v", err)
	}
	for _, name := range []string{"2026-08-28.jsonl", "keep.txt", "2026-99-99.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("unexpired/unrelated file removed: %s: %v", name, err)
		}
	}
	path := filepath.Join(dir, "2026-09-03.jsonl")
	if runtime.GOOS != "windows" {
		for path, mode := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("wrong permissions on %s: %v, %v", path, info, err)
			}
		}
	}
	// Fill the cap with complete JSON lines, rather than an incomplete zero tail.
	full := bytes.Repeat([]byte("{}\n"), usageDailyLimit/3)
	full = append(full, "{}\n"...)
	if err := os.WriteFile(path, full, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendUsage(dir, event); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != int64(len(full)) {
		t.Fatalf("daily cap not respected: %v, %v", info, err)
	}
	// The next UTC day gets a new file even when the previous day is full.
	event.Time = today.Add(24 * time.Hour).In(time.FixedZone("west", -7*60*60))
	if err := appendUsage(dir, event); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dir, "2026-09-04.jsonl")); err != nil || info.Size() == 0 {
		t.Fatalf("UTC day did not roll over: %v, %v", info, err)
	}
}

func TestUsageUnwritable(t *testing.T) {
	for _, blocker := range []string{"directory", "file", "permissions", "symlink"} {
		t.Run(blocker, func(t *testing.T) {
			dir := isolateUsage(t)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, time.Now().UTC().Format(time.DateOnly)+".jsonl")
			switch blocker {
			case "directory":
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dir, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if runtime.GOOS == "windows" || os.Geteuid() == 0 {
					t.Skip("requires Unix permission enforcement")
				}
				parent := filepath.Dir(dir)
				if err := os.Chmod(parent, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				t.Cleanup(func() {
					data, err := os.ReadFile(target)
					if err != nil || string(data) != "keep" {
						t.Errorf("symlink target changed: %q, %v", data, err)
					}
				})
			}
			for _, args := range [][]string{{"api", "describe", "courses.list"}, {"me", "SENSITIVE-extra"}} {
				wantOut, wantErr, wantExit := runUsage(t, false, args...)
				out, stderr, exit := runUsage(t, true, args...)
				if out != wantOut || stderr != wantErr || exit != wantExit {
					t.Fatalf("logging failure changed command output or exit")
				}
			}
		})
	}
}

func TestUsageConcurrentProcesses(t *testing.T) {
	dir := isolateUsage(t)
	// Preserve a complete existing record while concurrently repairing a partial tail.
	if _, _, exit := runUsage(t, true, "api", "describe", "courses.list"); exit != 0 {
		t.Fatal("could not seed log")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	file, err := os.OpenFile(paths[0], os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"unfinished":"` + strings.Repeat("x", 8192))
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			_, stderr, exit := runUsage(t, true, "api", "describe", "courses.list", "--json")
			if exit != 0 || stderr != "" {
				t.Errorf("command failed: exit=%d, stderr=%s", exit, stderr)
			}
		})
	}
	wg.Wait()
	if events := readUsage(t, dir); len(events) != count+1 {
		t.Fatalf("got %d complete records, want %d", len(events), count+1)
	}
}

func TestUsageVersion(t *testing.T) {
	for _, test := range []struct {
		info *debug.BuildInfo
		want string
	}{
		{nil, "devel"},
		{&debug.BuildInfo{}, "devel"},
		{&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "devel"},
		{&debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v1.2.3"},
		{&debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}}}, "abc123"},
	} {
		if got := usageVersion(test.info); got != test.want {
			t.Errorf("version = %q, want %q", got, test.want)
		}
	}
}
