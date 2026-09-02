package main

import "testing"

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
