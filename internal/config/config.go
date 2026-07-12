package config

import (
	"encoding/json"
	"errors"
	"fmt"
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
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return Config{}, errors.New("Canvas URL is required; set CANVAS_BASE_URL or use --base-url")
	}
	if !strings.HasPrefix(baseURL, "https://") && !strings.HasPrefix(baseURL, "http://") {
		return Config{}, fmt.Errorf("invalid Canvas URL %q: must start with http:// or https://", baseURL)
	}

	token := strings.TrimSpace(os.Getenv("CANVAS_API_TOKEN"))
	if token == "" {
		return Config{}, errors.New("Canvas access token is required; set CANVAS_API_TOKEN")
	}
	return Config{BaseURL: baseURL, Token: token}, nil
}

func SaveBaseURL(baseURL string) error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "canvas-cli")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(File{BaseURL: strings.TrimRight(baseURL, "/")}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "config.json"), append(data, '\n'), 0o600)
}
