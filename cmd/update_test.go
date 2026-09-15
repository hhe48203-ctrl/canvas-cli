package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUpdateProcess(t *testing.T) {
	if os.Getenv("CANVAS_UPDATE_TEST_HELPER") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "build":
		source, err := os.Open(os.Args[0])
		if err != nil {
			os.Exit(1)
		}
		defer source.Close()
		name := "canvas-cli"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		built, err := os.Create(filepath.Join(os.Getenv("GOBIN"), name))
		if err != nil {
			os.Exit(1)
		}
		_, err = io.Copy(built, source)
		_ = built.Close()
		if err != nil {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "build diagnostic")
	case "verify":
		fmt.Fprintln(os.Stdout, "verify diagnostic")
	case "fail":
		fmt.Fprintln(os.Stdout, "build failure")
		os.Exit(1)
	case "verify-fail":
		fmt.Fprintln(os.Stdout, "verify failure")
		os.Exit(1)
	}
	os.Exit(0)
}

func TestRootHasUpdateFlag(t *testing.T) {
	if flag := newRootCommand().Flags().Lookup("update"); flag == nil {
		t.Fatal("root command is missing --update")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "canvas")
	built := filepath.Join(dir, "built")
	if err := os.WriteFile(target, []byte("old"), 0o751); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(built, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(target, built); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("replacement = %q", data)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o751 {
		t.Fatalf("replacement mode = %o", info.Mode().Perm())
	}
}

func TestUpdateWritesStructuredSuccessWithoutDiagnosticOutput(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			resetCommandGlobals(t)
			prepareUpdateTest(t)
			var stderr bytes.Buffer
			root := newRootCommand()
			root.SetErr(&stderr)
			root.SetArgs([]string{"--" + format, "--update"})
			data, err := captureCommandOutput(t, root)
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				OK   bool `json:"ok" yaml:"ok"`
				Data struct {
					Message string `json:"message" yaml:"message"`
				} `json:"data" yaml:"data"`
			}
			if format == "json" {
				err = json.Unmarshal(data, &envelope)
			} else {
				err = yaml.Unmarshal(data, &envelope)
			}
			if err != nil || !envelope.OK || envelope.Data.Message != "Canvas updated successfully." || strings.Contains(string(data), "diagnostic") {
				t.Fatalf("output = %q, envelope = %#v, err = %v", data, envelope, err)
			}
			if strings.Count(string(data), "ok") != 1 || !strings.Contains(stderr.String(), "build diagnostic") || !strings.Contains(stderr.String(), "verify diagnostic") {
				t.Fatalf("stdout = %q, stderr = %q", data, stderr.String())
			}
		})
	}
}

func TestUpdateFailureReturnsError(t *testing.T) {
	resetCommandGlobals(t)
	prepareUpdateTest(t)
	updateCommand = func(string, ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=^TestUpdateProcess$", "--", "fail")
	}
	root := newRootCommand()
	root.SetArgs([]string{"--json", "--update"})
	if _, err := captureCommandOutput(t, root); err == nil || !strings.Contains(err.Error(), "build update") {
		t.Fatalf("update error = %v", err)
	}
}

func TestUpdateFailureWritesOneStructuredErrorEnvelope(t *testing.T) {
	for _, failure := range []struct{ phase, action, diagnostic string }{
		{"build", "build update", "build failure"},
		{"verify", "verify update", "verify failure"},
	} {
		t.Run(failure.phase, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "canvas")
			if err := os.WriteFile(target, []byte("original"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CANVAS_UPDATE_FAILURE_TEST_TARGET", target)
			t.Setenv("CANVAS_UPDATE_FAILURE_TEST_PHASE", failure.phase)

			for _, format := range []string{"json", "yaml"} {
				t.Run(format, func(t *testing.T) {
					stdout, stderr, exit := runUsage(t, false, "--"+format, "--update")
					if exit != 1 || stdout != "" {
						t.Fatalf("stdout = %q, stderr = %q, exit = %d", stdout, stderr, exit)
					}
					var envelope struct {
						OK    bool `json:"ok" yaml:"ok"`
						Error struct {
							Message string `json:"message" yaml:"message"`
						} `json:"error" yaml:"error"`
					}
					if format == "json" {
						if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
							t.Fatalf("JSON error envelope = %q, err = %v", stderr, err)
						}
					} else {
						decoder := yaml.NewDecoder(strings.NewReader(stderr))
						if err := decoder.Decode(&envelope); err != nil {
							t.Fatalf("YAML error envelope = %q, err = %v", stderr, err)
						}
						var extra any
						if err := decoder.Decode(&extra); err != io.EOF {
							t.Fatalf("YAML error output has extra document: %q, err = %v", stderr, err)
						}
					}
					if envelope.OK || !strings.Contains(envelope.Error.Message, failure.action) ||
						!strings.Contains(envelope.Error.Message, "Updating canvas...") || !strings.Contains(envelope.Error.Message, failure.diagnostic) {
						t.Fatalf("error envelope = %#v", envelope)
					}
				})
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
				t.Fatalf("target = %q, err = %v", data, err)
			}
		})
	}
}

func prepareUpdateTest(t *testing.T) {
	t.Helper()
	t.Setenv("CANVAS_UPDATE_TEST_HELPER", "1")
	dir := t.TempDir()
	target := filepath.Join(dir, "canvas")
	if err := os.WriteFile(target, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldExecutable, oldCommand, oldReplace := updateExecutable, updateCommand, updateReplace
	t.Cleanup(func() { updateExecutable, updateCommand, updateReplace = oldExecutable, oldCommand, oldReplace })
	updateExecutable = func() (string, error) { return target, nil }
	updateCommand = func(name string, _ ...string) *exec.Cmd {
		phase := "verify"
		if name == "go" {
			phase = "build"
		}
		return exec.Command(os.Args[0], "-test.run=^TestUpdateProcess$", "--", phase)
	}
	updateReplace = func(target, built string) error {
		if target == "" || built == "" {
			return fmt.Errorf("missing update path")
		}
		info, err := os.Stat(built)
		if err != nil {
			return fmt.Errorf("built update unavailable: %w", err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("built update is empty")
		}
		return nil
	}
}
