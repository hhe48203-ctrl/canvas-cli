package api

import "testing"

func TestBundledCatalogHasBroadCanvasCoverage(t *testing.T) {
	if len(GeneratedOperations) < 1000 {
		t.Fatalf("generated catalog has %d operations; want at least 1000", len(GeneratedOperations))
	}
	want := []struct {
		method, path string
	}{
		{"GET", "/api/v1/accounts"},
		{"GET", "/api/v1/courses/{course_id}/modules"},
		{"POST", "/api/v1/courses/{course_id}/pages"},
		{"GET", "/api/v1/groups/{group_id}/users"},
		{"GET", "/api/v1/users/{id}"},
	}
	for _, expected := range want {
		found := false
		for _, op := range GeneratedOperations {
			if op.Method == expected.method && op.Path == expected.path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("catalog missing %s %s", expected.method, expected.path)
		}
	}
}

func TestBundledCatalogIdentifiersAndRoutesAreUnique(t *testing.T) {
	ids := map[string]bool{}
	routes := map[string]bool{}
	for _, op := range GeneratedOperations {
		if ids[op.ID] {
			t.Errorf("duplicate operation ID %q", op.ID)
		}
		ids[op.ID] = true
		route := op.Method + " " + op.Path
		if routes[route] {
			t.Errorf("duplicate route %q", route)
		}
		routes[route] = true
	}
}

func TestGeneratedOperationsContainUsefulHelpMetadata(t *testing.T) {
	for _, op := range GeneratedOperations {
		if op.ID == "context_modules_api.create" {
			if op.Summary == "" || op.Group == "" || op.DocsURL == "" || len(op.Scopes) == 0 {
				t.Fatalf("operation metadata is incomplete: %#v", op)
			}
			if len(op.ParametersIn("path")) == 0 || op.RequestBody == nil || len(op.RequestBody.ContentTypes) == 0 {
				t.Fatalf("operation invocation metadata is incomplete: %#v", op)
			}
			return
		}
	}
	t.Fatal("context_modules_api.create not found")
}
