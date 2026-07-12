package canvas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func NewClient(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) Request(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (Response, error) {
	return c.RequestWithHeaders(ctx, method, path, query, body, contentType, nil)
}

func (c *Client) RequestWithHeaders(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, headers http.Header) (Response, error) {
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		path = c.BaseURL + "/" + strings.TrimLeft(path, "/")
	}
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return Response{}, err
	}
	if query != nil {
		merged := req.URL.Query()
		for key, values := range query {
			for _, value := range values {
				merged.Add(key, value)
			}
		}
		req.URL.RawQuery = merged.Encode()
	}
	req.Header.Set("Accept", "application/json")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if c.Token != "" && req.URL.Host == mustURLHost(c.BaseURL) {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	result := Response{StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), Body: data}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, &HTTPError{StatusCode: resp.StatusCode, Body: data}
	}
	return result, nil
}

func (c *Client) JSON(ctx context.Context, method, path string, query url.Values, payload any) (Response, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return Response{}, err
		}
		body = bytes.NewReader(data)
	}
	return c.Request(ctx, method, path, query, body, "application/json")
}

func (c *Client) Form(ctx context.Context, method, path string, values url.Values) (Response, error) {
	return c.Request(ctx, method, path, nil, strings.NewReader(values.Encode()), "application/x-www-form-urlencoded")
}

func (c *Client) Download(ctx context.Context, path, destination string) (int64, error) {
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		path = c.BaseURL + "/" + strings.TrimLeft(path, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "*/*")
	if c.Token != "" && req.URL.Host == mustURLHost(c.BaseURL) {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return 0, readErr
		}
		return 0, &HTTPError{StatusCode: resp.StatusCode, Body: data}
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return io.Copy(file, resp.Body)
}

func (c *Client) Upload(ctx context.Context, endpoint, filePath string) (map[string]any, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	metadata := url.Values{
		"name":         {filepath.Base(filePath)},
		"size":         {strconv.FormatInt(info.Size(), 10)},
		"content_type": {contentType(filePath)},
	}
	first, err := c.Form(ctx, http.MethodPost, endpoint, metadata)
	if err != nil {
		return nil, err
	}
	var upload struct {
		UploadURL    string         `json:"upload_url"`
		UploadParams map[string]any `json:"upload_params"`
	}
	if err := json.Unmarshal(first.Body, &upload); err != nil {
		return nil, fmt.Errorf("Canvas upload initialization returned invalid JSON: %w", err)
	}
	if upload.UploadURL == "" {
		return nil, fmt.Errorf("Canvas upload initialization did not return upload_url")
	}

	result, err := multipartUpload(ctx, c, upload.UploadURL, upload.UploadParams, filePath)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if len(result) > 0 {
		if err := json.Unmarshal(result, &payload); err != nil {
			return nil, fmt.Errorf("Canvas upload returned invalid JSON: %w", err)
		}
	}
	return payload, nil
}

func multipartUpload(ctx context.Context, c *Client, endpoint string, params map[string]any, filePath string) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range params {
		if err := writer.WriteField(key, fmt.Sprint(value)); err != nil {
			return nil, err
		}
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Canvas file uploads may redirect from external storage back to Canvas. Stop
	// automatic redirects so the confirmation request can be authenticated.
	uploadClient := *c.HTTPClient
	uploadClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := uploadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if (resp.StatusCode >= 300 && resp.StatusCode < 400) || resp.StatusCode == http.StatusCreated {
		location := resp.Header.Get("Location")
		if location == "" && resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Canvas upload returned %d without Location", resp.StatusCode)
		}
		if location != "" {
			final, err := c.Request(ctx, http.MethodGet, location, nil, nil, "")
			if err != nil {
				return nil, err
			}
			return final.Body, nil
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: data}
	}
	return data, nil
}

// NextLink returns the opaque next-page URL from an RFC 8288 Link header.
func NextLink(header http.Header) string {
	for _, part := range splitLinkHeader(header.Values("Link")) {
		sections := strings.Split(part, ";")
		if len(sections) < 2 {
			continue
		}
		urlPart := strings.TrimSpace(sections[0])
		if !strings.HasPrefix(urlPart, "<") || !strings.HasSuffix(urlPart, ">") {
			continue
		}
		for _, parameter := range sections[1:] {
			name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(name, "rel") && strings.Trim(value, `"`) == "next" {
				return strings.TrimSuffix(strings.TrimPrefix(urlPart, "<"), ">")
			}
		}
	}
	return ""
}

func splitLinkHeader(values []string) []string {
	var result []string
	for _, value := range values {
		start := 0
		inAngles := false
		for i, r := range value {
			switch r {
			case '<':
				inAngles = true
			case '>':
				inAngles = false
			case ',':
				if !inAngles {
					result = append(result, value[start:i])
					start = i + 1
				}
			}
		}
		result = append(result, value[start:])
	}
	return result
}

func contentType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".doc", ".docx":
		return "application/msword"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func mustURLHost(raw string) string {
	u, _ := url.Parse(raw)
	return u.Host
}

type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	message := strings.TrimSpace(string(e.Body))
	if len(message) > 500 {
		message = message[:500]
	}
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("Canvas API returned HTTP %d: %s", e.StatusCode, message)
}
