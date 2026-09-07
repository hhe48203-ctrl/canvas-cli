package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDefaultAcceptsLiteralValues(t *testing.T) {
	tests := []struct {
		description, typeName, expected string
		enum                            []string
	}{
		{description: "Defaults to false if omitted.", typeName: "boolean", expected: "false"},
		{description: "Defaults to 'AccountAdmin'.", typeName: "string", expected: "AccountAdmin"},
		{description: `Defaults to "name"`, typeName: "string", expected: "name"},
		{description: "Default: 10; maximum 100.", typeName: "integer", expected: "10"},
		{description: "Defaults to warning when omitted.", typeName: "string", enum: []string{"warning", "error"}, expected: "warning"},
		{description: "Defaults to 14 days ago. Use ISO 8601.", typeName: "string", expected: "14 days ago"},
	}
	for _, test := range tests {
		if got := parseDefault(test.description, test.typeName, test.enum); got != test.expected {
			t.Errorf("parseDefault(%q) = %q; want %q", test.description, got, test.expected)
		}
	}
}

func TestParseDefaultRejectsProseDescriptions(t *testing.T) {
	for _, description := range []string{
		`Defaults to the domain root account ("self").`,
		"Defaults to the current user.",
	} {
		if got := parseDefault(description, "string", nil); got != "" {
			t.Errorf("parseDefault(%q) = %q; want no inferred default", description, got)
		}
	}
}

func TestNormalizeTypePreservesCanvasNumericTypes(t *testing.T) {
	for _, input := range []string{"number", "Numeric", "Float", "Decimal"} {
		if got := normalizeType(input); got != "number" {
			t.Errorf("normalizeType(%q) = %q; want number", input, got)
		}
	}
	if got := normalizeType("Positive Integer"); got != "integer" {
		t.Errorf("normalizeType(Positive Integer) = %q; want integer", got)
	}
}

func TestGeneratorRejectsEmptyDocumentationWithoutReplacingOutput(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "error.html")
	outPath := filepath.Join(dir, "catalog.yaml")
	if err := os.WriteFile(htmlPath, []byte("<html><title>Access denied</title></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("existing catalog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", ".", "-html", htmlPath, "-out", outPath)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "no API paths extracted") {
		t.Fatalf("generator error = %v, output = %s", err, output)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "existing catalog\n" {
		t.Fatalf("output changed to %q", got)
	}
}

func TestGeneratorWritesExtractedPaths(t *testing.T) {
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "docs.html")
	outPath := filepath.Join(dir, "catalog.yaml")
	html := `<div class="method_details"><h2 class='api_method_name' name='method.courses.index' data-subtopic='Courses'><a>List courses</a></h2><h3 class='endpoint'>GET /api/v1/courses</h3></div>`
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("go", "run", ".", "-html", htmlPath, "-out", outPath).CombinedOutput(); err != nil {
		t.Fatalf("generator failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/api/v1/courses:") {
		t.Fatalf("generated catalog missing path: %s", data)
	}
}
