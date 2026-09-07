package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCollectOperationsMergesPathParametersAndReferences(t *testing.T) {
	input := []byte(`openapi: 3.0.0
components:
  parameters:
    CourseID:
      name: course_id
      in: path
      required: true
      description: Canvas course ID
      schema: {type: string}
  requestBodies:
    Page:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/PageInput'
  schemas:
    PageInput:
      type: object
      required: [title]
      properties:
        title: {type: string, description: Page title}
        published: {type: boolean, default: false}
  responses:
    Created:
      description: Created
      content:
        application/json: {}
security:
  - oauth2: [pages:write]
paths:
  /api/v1/courses/{course_id}/pages:
    parameters:
      - $ref: '#/components/parameters/CourseID'
    post:
      operationId: pages.create
      summary: Create page
      parameters:
        - name: notify
          in: query
          schema: {type: boolean, default: false}
      requestBody:
        $ref: '#/components/requestBodies/Page'
      responses:
        '200':
          $ref: '#/components/responses/Created'
`)
	var spec document
	if err := yaml.Unmarshal(input, &spec); err != nil {
		t.Fatal(err)
	}
	items := collectOperations(spec)
	if len(items) != 1 {
		t.Fatalf("operations = %d", len(items))
	}
	if len(items[0].params) != 2 || items[0].params[0].Name != "course_id" || !items[0].params[0].Required {
		t.Fatalf("parameters = %#v", items[0].params)
	}
	body := resolveRequestBody(spec, items[0].op.RequestBody)
	if body == nil || !body.Required {
		t.Fatalf("request body = %#v", body)
	}
	bodyParams := bodyParameters(spec, body)
	if len(bodyParams) != 2 || bodyParams[1].Name != "title" || !bodyParams[1].Required {
		t.Fatalf("body parameters = %#v", bodyParams)
	}
	if got := resolveResponse(spec, items[0].op.Responses["200"]); got.Description != "Created" {
		t.Fatalf("response = %#v", got)
	}
	if len(items[0].op.Security) != 1 || items[0].op.Security[0]["oauth2"][0] != "pages:write" {
		t.Fatalf("security = %#v", items[0].op.Security)
	}
}

func TestGeneratorRejectsEmptyPathsWithoutReplacingOutput(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "empty.yaml")
	outPath := filepath.Join(dir, "generated.go")
	if err := os.WriteFile(specPath, []byte("openapi: 3.0.0\npaths: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("existing generated catalog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("go", "run", ".", "-spec", specPath, "-out", outPath).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "no API operations found") {
		t.Fatalf("generator error = %v, output = %s", err, output)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "existing generated catalog\n" {
		t.Fatalf("output changed to %q", got)
	}
}

func TestGeneratorWritesOperations(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "catalog.yaml")
	outPath := filepath.Join(dir, "generated.go")
	spec := "openapi: 3.0.0\npaths:\n  /api/v1/courses:\n    get:\n      operationId: courses.index\n"
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("go", "run", ".", "-spec", specPath, "-out", outPath).CombinedOutput(); err != nil {
		t.Fatalf("generator failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `ID: "courses.index"`) {
		t.Fatalf("generated catalog missing operation: %s", data)
	}
}
