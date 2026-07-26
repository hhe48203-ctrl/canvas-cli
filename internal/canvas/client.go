package canvas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
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
	if c.Token != "" && sameOrigin(req.URL, parsedURL(c.BaseURL)) {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.do(req)
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
	if c.Token != "" && sameOrigin(req.URL, parsedURL(c.BaseURL)) {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.do(req)
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
		decoder := json.NewDecoder(bytes.NewReader(result))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return nil, fmt.Errorf("Canvas upload returned invalid JSON: %w", err)
		}
	}
	return payload, nil
}

func multipartUpload(ctx context.Context, c *Client, endpoint string, params map[string]any, filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	boundary, overhead, err := multipartEnvelope(params, filepath.Base(filePath))
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.Size() > math.MaxInt64-overhead {
		_ = file.Close()
		return nil, fmt.Errorf("file is too large to upload")
	}

	reader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	if err := writer.SetBoundary(boundary); err != nil {
		_ = file.Close()
		_ = reader.Close()
		_ = pipeWriter.Close()
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		_ = file.Close()
		_ = reader.Close()
		_ = pipeWriter.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = overhead + info.Size()

	writeDone := make(chan error, 1)
	go func() {
		writeErr := writeMultipart(writer, params, filepath.Base(filePath), file)
		_ = pipeWriter.CloseWithError(writeErr)
		writeDone <- writeErr
	}()

	// Canvas file uploads may redirect from external storage back to Canvas. Stop
	// automatic redirects so the confirmation request can be authenticated.
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	uploadClient := *httpClient
	uploadClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := uploadClient.Do(req)
	_ = reader.Close()
	writeErr := <-writeDone
	if err != nil {
		return nil, err
	}
	if writeErr != nil && !errors.Is(writeErr, io.ErrClosedPipe) {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("stream upload: %w", writeErr)
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

type byteCounter int64

func (counter *byteCounter) Write(data []byte) (int, error) {
	*counter += byteCounter(len(data))
	return len(data), nil
}

func multipartEnvelope(params map[string]any, fileName string) (string, int64, error) {
	var size byteCounter
	writer := multipart.NewWriter(&size)
	if err := writeMultipartFields(writer, params); err != nil {
		return "", 0, err
	}
	if _, err := writer.CreateFormFile("file", fileName); err != nil {
		return "", 0, err
	}
	if err := writer.Close(); err != nil {
		return "", 0, err
	}
	return writer.Boundary(), int64(size), nil
}

func writeMultipart(writer *multipart.Writer, params map[string]any, fileName string, file *os.File) error {
	if err := writeMultipartFields(writer, params); err != nil {
		_ = file.Close()
		return err
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err == nil {
		_, err = io.Copy(part, file)
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeMultipartFields(writer *multipart.Writer, params map[string]any) error {
	for key, value := range params {
		if err := writer.WriteField(key, fmt.Sprint(value)); err != nil {
			return err
		}
	}
	return nil
}

// NextLink returns the opaque next-page URL from an RFC 8288 Link header.
func NextLink(header http.Header) string {
	var linkValues []string
	for name, values := range header {
		if strings.EqualFold(name, "Link") {
			linkValues = append(linkValues, values...)
		}
	}
	for _, part := range splitLinkHeader(linkValues) {
		part = strings.TrimSpace(part)
		end := strings.IndexByte(part, '>')
		if !strings.HasPrefix(part, "<") || end < 1 {
			continue
		}
		for _, parameter := range splitOutside(part[end+1:], ';', false) {
			name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || !strings.EqualFold(name, "rel") {
				continue
			}
			value = strings.TrimSpace(value)
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
			for _, relation := range strings.Fields(value) {
				if strings.EqualFold(relation, "next") {
					return part[1:end]
				}
			}
		}
	}
	return ""
}

func splitLinkHeader(values []string) []string {
	var result []string
	for _, value := range values {
		result = append(result, splitOutside(value, ',', true)...)
	}
	return result
}

func splitOutside(value string, separator byte, trackAngles bool) []string {
	var result []string
	start := 0
	inAngles := false
	inQuotes := false
	escaped := false
	for i := 0; i < len(value); i++ {
		char := value[i]
		if escaped {
			escaped = false
			continue
		}
		if inQuotes && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' && !inAngles {
			inQuotes = !inQuotes
			continue
		}
		if trackAngles && !inQuotes {
			switch char {
			case '<':
				inAngles = true
			case '>':
				inAngles = false
			}
		}
		if char == separator && !inAngles && !inQuotes {
			result = append(result, value[start:i])
			start = i + 1
		}
	}
	return append(result, value[start:])
}

func contentType(path string) string {
	detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if detected == "" {
		return "application/octet-stream"
	}
	mediaType, _, err := mime.ParseMediaType(detected)
	if err != nil {
		return detected
	}
	return mediaType
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	redirectSafeClient := *httpClient
	previousCheck := redirectSafeClient.CheckRedirect
	base := parsedURL(c.BaseURL)
	redirectSafeClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !sameOrigin(next.URL, base) {
			next.Header.Del("Authorization")
		}
		if previousCheck != nil {
			return previousCheck(next, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return redirectSafeClient.Do(req)
}

func parsedURL(raw string) *url.URL {
	parsed, _ := url.Parse(raw)
	return parsed
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil ||
		!strings.EqualFold(left.Scheme, right.Scheme) ||
		!strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return originPort(left) == originPort(right)
}

func originPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
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
