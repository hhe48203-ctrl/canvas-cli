package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type File struct {
	BaseURL string `json:"base_url,omitempty"`
}

type Config struct {
	BaseURL string
	Token   string
}

func Resolve(baseURL string) (Config, error) {
	file := File{}
	if dir, err := os.UserConfigDir(); err == nil {
		path := filepath.Join(dir, "canvas-cli", "config.json")
		if data, readErr := os.ReadFile(path); readErr == nil {
			_ = json.Unmarshal(data, &file)
		}
	}

	if baseURL == "" {
		baseURL = os.Getenv("CANVAS_BASE_URL")
	}
	if baseURL == "" {
		baseURL = file.BaseURL
	}
	normalizedBaseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return Config{}, err
	}

	token := strings.TrimSpace(os.Getenv("CANVAS_API_TOKEN"))
	if token == "" {
		return Config{}, errors.New("Canvas access token is required; set CANVAS_API_TOKEN")
	}
	return Config{BaseURL: normalizedBaseURL, Token: token}, nil
}

func SaveBaseURL(baseURL string) error {
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "canvas-cli")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(File{BaseURL: baseURL}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "config.json"), append(data, '\n'), 0o600)
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Canvas URL is required; set CANVAS_BASE_URL or use --base-url")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid Canvas URL %q: %w", raw, err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "http") {
		return "", fmt.Errorf("invalid Canvas URL %q: scheme must be http or https", raw)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid Canvas URL %q: host is required", raw)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("invalid Canvas URL %q: user information is not allowed", raw)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid Canvas URL %q: query and fragment are not allowed", raw)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return parsed.String(), nil
}
