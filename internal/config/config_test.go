package config

import (
	"strings"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	valid := map[string]string{
		" https://canvas.test/ ":      "https://canvas.test",
		"HTTPS://CANVAS.test/canvas/": "https://CANVAS.test/canvas",
		"http://localhost:3000":       "http://localhost:3000",
	}
	for input, expected := range valid {
		got, err := normalizeBaseURL(input)
		if err != nil {
			t.Errorf("normalizeBaseURL(%q): %v", input, err)
		} else if got != expected {
			t.Errorf("normalizeBaseURL(%q) = %q; want %q", input, got, expected)
		}
	}
}

func TestNormalizeBaseURLRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	tests := map[string]string{
		"canvas.test":                      "scheme",
		"ftp://canvas.test":                "scheme",
		"https:///api/v1":                  "host",
		"https://user@canvas.test":         "user information",
		"https://canvas.test?redirect=x":   "query and fragment",
		"https://canvas.test/api#fragment": "query and fragment",
	}
	for input, expected := range tests {
		_, err := normalizeBaseURL(input)
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Errorf("normalizeBaseURL(%q) error = %v; want text %q", input, err, expected)
		}
	}
}

func TestResolveReturnsNormalizedBaseURL(t *testing.T) {
	t.Setenv("CANVAS_API_TOKEN", " token ")
	config, err := Resolve(" https://canvas.test/canvas/ ")
	if err != nil {
		t.Fatal(err)
	}
	if config.BaseURL != "https://canvas.test/canvas" || config.Token != "token" {
		t.Fatalf("config = %#v", config)
	}
}

func TestResolveBaseURLPrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CANVAS_BASE_URL", "https://environment.test/canvas")
	if err := SaveBaseURL("https://saved.test/canvas"); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveBaseURL("https://flag.test/canvas"); err != nil || got != "https://flag.test/canvas" {
		t.Fatalf("flag URL = %q, %v", got, err)
	}
	if got, err := ResolveBaseURL(""); err != nil || got != "https://environment.test/canvas" {
		t.Fatalf("environment URL = %q, %v", got, err)
	}
	t.Setenv("CANVAS_BASE_URL", "")
	if got, err := ResolveBaseURL(""); err != nil || got != "https://saved.test/canvas" {
		t.Fatalf("saved URL = %q, %v", got, err)
	}
}
