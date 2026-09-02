package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesDownloadUsesCanonicalFileRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/files/123/download"; got != want {
			t.Fatalf("download path = %q; want %q", got, want)
		}
		_, _ = w.Write([]byte("lecture"))
	}))
	defer server.Close()

	oldBaseURL, oldFormat, oldJSON, oldYAML, oldDownloadPath := baseURL, format, jsonOutput, yamlOutput, downloadPath
	baseURL, format, jsonOutput, yamlOutput, downloadPath = "", "", false, false, ""
	t.Cleanup(func() {
		baseURL, format, jsonOutput, yamlOutput, downloadPath = oldBaseURL, oldFormat, oldJSON, oldYAML, oldDownloadPath
	})
	t.Setenv("CANVAS_BASE_URL", server.URL)
	t.Setenv("CANVAS_API_TOKEN", "token")

	destination := filepath.Join(t.TempDir(), "lecture.pdf")
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = oldStdout })

	root := newRootCommand()
	root.SetArgs([]string{"--json", "files", "download", "123", "--destination", destination})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	_, _ = io.ReadAll(reader)

	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "lecture" {
		t.Fatalf("downloaded data = %q", data)
	}
}
