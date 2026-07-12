package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestTableUnwrapsSuccessEnvelope(t *testing.T) {
	var buffer bytes.Buffer
	value := Success([]map[string]any{{"id": 1, "name": "Course", "workflow_state": "available"}})
	if err := PrintTo(&buffer, value, "table"); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"ID", "Course", "available"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Fatalf("table output %q does not contain %q", buffer.String(), expected)
		}
	}
}

func TestFailureEnvelopeIsStructuredJSON(t *testing.T) {
	var buffer bytes.Buffer
	if err := PrintTo(&buffer, Failure(assertionError("bad request")), "json"); err != nil {
		t.Fatal(err)
	}
	if got := buffer.String(); !strings.Contains(got, `"ok":false`) || !strings.Contains(got, `"bad request"`) {
		t.Fatalf("JSON error output = %q", got)
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
