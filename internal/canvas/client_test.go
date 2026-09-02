package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientDoesNotPutATotalTimeoutOnFileTransfers(t *testing.T) {
	client := NewClient("https://canvas.test", "secret")
	if client.HTTPClient.Timeout != 0 {
		t.Fatalf("client timeout = %s; want no total transfer timeout", client.HTTPClient.Timeout)
	}
	transport, ok := client.HTTPClient.Transport.(*http.Transport)
	if !ok || transport.ResponseHeaderTimeout != 60*time.Second {
		t.Fatalf("transport = %#v; want a 60-second response header timeout", client.HTTPClient.Transport)
	}
}

func TestRequestAddsBearerTokenAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("page = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, "secret")
	resp, err := c.Request(context.Background(), http.MethodGet, "/api/v1/courses", url.Values{"page": {"2"}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("body = %s", resp.Body)
	}
}

func TestRequestMergesEmbeddedAndExplicitQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("embedded"); got != "yes" {
			t.Fatalf("embedded query = %q", got)
		}
		if got := r.URL.Query().Get("explicit"); got != "ok" {
			t.Fatalf("explicit query = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "secret").Request(
		context.Background(), http.MethodGet, server.URL+"/api/v1/test?embedded=yes",
		url.Values{"explicit": {"ok"}}, nil, "",
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRequestAuthenticatesOnlyTheConfiguredCanvasOrigin(t *testing.T) {
	tests := []struct {
		name, target, authorization string
	}{
		{"canonical origin", "https://canvas.test:443/api/v1/courses", "Bearer secret"},
		{"scheme downgrade", "http://canvas.test/api/v1/courses", ""},
		{"different port", "https://canvas.test:444/api/v1/courses", ""},
		{"different host", "https://files.test/api/v1/courses", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient("https://CANVAS.test", "secret")
			client.HTTPClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if got := request.Header.Get("Authorization"); got != test.authorization {
					t.Fatalf("authorization = %q; want %q", got, test.authorization)
				}
				return testResponse(request, http.StatusOK, nil), nil
			})
			if _, err := client.Request(context.Background(), http.MethodGet, test.target, nil, nil, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequestStripsAuthorizationOnCrossOriginRedirect(t *testing.T) {
	client := NewClient("https://canvas.test", "secret")
	requests := 0
	client.HTTPClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if got := request.Header.Get("Authorization"); got != "Bearer secret" {
				t.Fatalf("initial authorization = %q", got)
			}
			return testResponse(request, http.StatusFound, http.Header{"Location": {"http://canvas.test/download"}}), nil
		case 2:
			if got := request.Header.Get("Authorization"); got != "" {
				t.Fatalf("redirect authorization = %q", got)
			}
			return testResponse(request, http.StatusOK, nil), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})
	if _, err := client.Request(context.Background(), http.MethodGet, "/api/v1/files/1/download", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestNextLink(t *testing.T) {
	header := http.Header{"Link": {`<https://canvas.test/api/v1/courses?page=1>; rel="current", <https://canvas.test/api/v1/courses?opaque=a,b>; rel="next"`}}
	if got := NextLink(header); got != "https://canvas.test/api/v1/courses?opaque=a,b" {
		t.Fatalf("next link = %q", got)
	}
}

func TestNextLinkHandlesRFCRelationsAndDelimiters(t *testing.T) {
	header := http.Header{"link": {
		`<https://canvas.test/api/v1/courses?opaque=a,b;c>; title="page, two; ready"; rel="prev next"`,
	}}
	if got := NextLink(header); got != "https://canvas.test/api/v1/courses?opaque=a,b;c" {
		t.Fatalf("next link = %q", got)
	}
}

func TestRequestReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "secret").Request(context.Background(), http.MethodGet, "/api/v1/courses", nil, nil, "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected HTTP error, got %v", err)
	}
}

func TestDownloadDoesNotReplaceDestinationWithPartialContent(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "lecture.pdf")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := NewClient("https://canvas.test", "secret")
	client.HTTPClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := testResponse(request, http.StatusOK, nil)
		response.Body = io.NopCloser(io.MultiReader(strings.NewReader("partial"), failingReader{}))
		return response, nil
	})
	if _, err := client.Download(context.Background(), "/api/v1/files/1/download", destination); err == nil {
		t.Fatal("expected the interrupted download to fail")
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("destination = %q", data)
	}
	temporary, err := filepath.Glob(filepath.Join(directory, ".lecture.pdf.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporary files remain: %#v", temporary)
	}
}

func TestDownloadAtomicallyReplacesDestination(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "lecture.pdf")
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := NewClient("https://canvas.test", "secret")
	client.HTTPClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := testResponse(request, http.StatusOK, nil)
		response.Body = io.NopCloser(strings.NewReader("complete"))
		return response, nil
	})
	written, err := client.Download(context.Background(), "/api/v1/files/1/download", destination)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len("complete")) || string(data) != "complete" {
		t.Fatalf("written = %d, destination = %q", written, data)
	}
}

func TestUploadCompletesMultipartFlow(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "notes.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(nil)
	defer server.Close()

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/courses/1/files":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("name") != "notes.txt" {
				t.Fatalf("name = %q", r.Form.Get("name"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"upload_url":"` + server.URL + `/upload","upload_params":{"token":"x"}}`))
		case "/upload":
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42}`))
		default:
			http.NotFound(w, r)
		}
	})

	data, err := NewClient(server.URL, "secret").Upload(context.Background(), "/api/v1/courses/1/files", filePath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(data["id"]) != "42" {
		t.Fatalf("upload result = %#v", data)
	}
}

func TestMultipartUploadStreamsTheFileAsTheLastPart(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "notes.txt")
	if err := os.WriteFile(filePath, []byte("streamed"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := NewClient("https://canvas.test", "secret")
	client.HTTPClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.ContentLength <= int64(len("streamed")) {
			t.Fatalf("content length = %d", request.ContentLength)
		}
		if _, ok := request.Body.(*io.PipeReader); !ok {
			t.Fatalf("request body type = %T; want a streaming pipe", request.Body)
		}
		multipartReader, err := request.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for {
			part, err := multipartReader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, part.FormName())
			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			if part.FormName() == "file" && string(data) != "streamed" {
				t.Fatalf("file data = %q", data)
			}
		}
		if got := strings.Join(names, ","); got != "key,file" {
			t.Fatalf("multipart fields = %q", got)
		}
		response := testResponse(request, http.StatusOK, nil)
		response.Body = io.NopCloser(strings.NewReader(`{"id":42}`))
		return response, nil
	})

	result, err := multipartUpload(context.Background(), client, "https://storage.test/upload", map[string]any{"key": "opaque"}, filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"id":42}` {
		t.Fatalf("result = %q", result)
	}
}

func TestUploadPreservesCanvasIntegerIDs(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "notes.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(nil)
	defer server.Close()
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/courses/1/files":
			_, _ = w.Write([]byte(`{"upload_url":"` + server.URL + `/upload","upload_params":{}}`))
		case "/upload":
			_, _ = w.Write([]byte(`{"id":9007199254740993}`))
		default:
			http.NotFound(w, r)
		}
	})

	data, err := NewClient(server.URL, "secret").Upload(context.Background(), "/api/v1/courses/1/files", filePath)
	if err != nil {
		t.Fatal(err)
	}
	if data["id"] != json.Number("9007199254740993") {
		t.Fatalf("upload id = %#v; want an exact JSON number", data["id"])
	}
}

func TestUploadContentTypeUsesStandardMIMETypes(t *testing.T) {
	tests := map[string]string{
		"essay.DOCX": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"image.PNG":  "image/png",
		"notes.txt":  "text/plain",
		"data.abcx":  "application/octet-stream",
	}
	for path, expected := range tests {
		if got := contentType(path); got != expected {
			t.Errorf("contentType(%q) = %q; want %q", path, got, expected)
		}
	}
}

func TestUploadFollowsRedirectWithCanvasAuthentication(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusCreated} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			tmp := t.TempDir()
			filePath := filepath.Join(tmp, "notes.txt")
			if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
				t.Fatal(err)
			}

			var canvasServer *httptest.Server
			storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "" {
					t.Fatalf("storage received authorization %q", got)
				}
				w.Header().Set("Location", canvasServer.URL+"/api/v1/files/42/create_success")
				w.WriteHeader(status)
			}))
			defer storageServer.Close()

			canvasServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/courses/1/files":
					_, _ = w.Write([]byte(`{"upload_url":"` + storageServer.URL + `","upload_params":{"key":"opaque"}}`))
				case "/api/v1/files/42/create_success":
					if got := r.Header.Get("Authorization"); got != "Bearer secret" {
						t.Fatalf("confirmation authorization = %q", got)
					}
					_, _ = w.Write([]byte(`{"id":42}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer canvasServer.Close()

			data, err := NewClient(canvasServer.URL, "secret").Upload(context.Background(), "/api/v1/courses/1/files", filePath)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(data["id"]) != "42" {
				t.Fatalf("upload result = %#v", data)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testResponse(request *http.Request, status int, header http.Header) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    request,
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("download interrupted")
}
