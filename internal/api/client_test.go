package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoSendsMethodPathQueryAndAuthHeader(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "secrettoken", 5*time.Second, nil)
	q := map[string][]string{"state": {"open"}, "limit": {"3"}}
	var out struct {
		OK bool `json:"ok"`
	}
	err := c.Do("GET", "/repos/o/r/pulls", q, nil, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if gotMethod != "GET" {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if want := "/api/v1/repos/o/r/pulls"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotRawQuery != "limit=3&state=open" {
		t.Errorf("query = %q, want limit=3&state=open", gotRawQuery)
	}
	if gotAuth != "token secrettoken" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "token secrettoken")
	}
	if !out.OK {
		t.Error("response body not decoded into out")
	}
}

func TestDoSetsContentTypeOnPostBody(t *testing.T) {
	var gotBody string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", time.Second, nil)
	body := map[string]string{"title": "hi"}
	if err := c.Do("POST", "/repos/o/r/pulls", nil, body, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("request body not JSON: %v (%q)", err, gotBody)
	}
	if sent["title"] != "hi" {
		t.Errorf("body title = %q, want hi", sent["title"])
	}
}

func TestDo404DecodesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"x"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", time.Second, nil)
	err := c.Do("GET", "/repos/o/r/nope", nil, nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != 404 || apiErr.Message != "x" {
		t.Errorf("APIError = %+v, want {404 x}", *apiErr)
	}
	if apiErr.Error() != "404: x" {
		t.Errorf("Error() = %q, want %q", apiErr.Error(), "404: x")
	}
}

func TestDoNon2xxWithoutMessageFallsBackToStatusText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", time.Second, nil)
	err := c.Do("GET", "/boom", nil, nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != 500 || apiErr.Message != http.StatusText(500) {
		t.Errorf("APIError = %+v, want {500 Internal Server Error}", *apiErr)
	}
}

func TestDoNetworkFailureIsTagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens anymore

	c := NewClient(url, "", 2*time.Second, nil)
	err := c.Do("GET", "/repos/o/r/pulls", nil, nil, nil)
	if err == nil {
		t.Fatal("want error for unreachable server")
	}
	if !IsNetwork(err) {
		t.Errorf("IsNetwork(err) = false, want true (err: %v)", err)
	}
}

func TestDoVerboseLoggingOmitsHeaders(t *testing.T) {
	var lines []string
	log := testLogger{lines: &lines}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "supersecret", time.Second, log)
	if err := c.Do("GET", "/repos/o/r/pulls", nil, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected verbose log line")
	}
	for _, l := range lines {
		for _, banned := range []string{"supersecret", "Authorization"} {
			if strings.Contains(l, banned) {
				t.Errorf("log line leaks %q: %s", banned, l)
			}
		}
	}
}

type testLogger struct{ lines *[]string }

func (l testLogger) Logf(format string, args ...any) {
	*l.lines = append(*l.lines, fmt.Sprintf(format, args...))
}

func TestDoRawTextResponse(t *testing.T) {
	const patch = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n"
	var gotMethod, gotPath, gotAccept, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(patch))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "secrettoken", 5*time.Second, nil)
	raw, err := c.DoRaw("GET", "/repos/o/r/pulls/5.diff", nil, nil)
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/api/v1/repos/o/r/pulls/5.diff" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if raw.Status != 200 {
		t.Errorf("Status = %d, want 200", raw.Status)
	}
	if raw.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("ContentType = %q", raw.ContentType)
	}
	if string(raw.Body) != patch {
		t.Errorf("Body = %q, want exact patch bytes", raw.Body)
	}
	if gotAccept != "" {
		t.Errorf("Accept = %q, want empty so text endpoints keep their representation", gotAccept)
	}
	if gotAuth != "token secrettoken" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestDoRawSendsBodyAndContentType(t *testing.T) {
	var gotBody string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", time.Second, nil)
	payload := []byte(`{"hello":"world"}`)
	if _, err := c.DoRaw("POST", "/repos/o/r/anything", nil, payload); err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody != string(payload) {
		t.Errorf("body = %q, want verbatim %q", gotBody, payload)
	}
}

func TestDoRawNon2xxDecodesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"message":"merge conflict"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", time.Second, nil)
	_, err := c.DoRaw("GET", "/repos/o/r/pulls/5.diff", nil, nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != 409 || apiErr.Message != "merge conflict" {
		t.Errorf("APIError = %+v, want {409 merge conflict}", *apiErr)
	}
}

func TestDoRawNetworkFailureIsTagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens anymore

	c := NewClient(url, "", 2*time.Second, nil)
	_, err := c.DoRaw("GET", "/repos/o/r/pulls/5.diff", nil, nil)
	if err == nil {
		t.Fatal("want error for unreachable server")
	}
	if !IsNetwork(err) {
		t.Errorf("IsNetwork(err) = false, want true (err: %v)", err)
	}
}

func TestDoStillDecodesAndSetsJSONHeaders(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", time.Second, nil)
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Do("GET", "/repos/o/r/pulls", nil, nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if !out.OK {
		t.Error("response body not decoded into out")
	}
}

func TestDownloadSendsTokenAndReturnsBinaryBody(t *testing.T) {
	var gotPath, gotAuth, gotAccept string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotAccept = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept")
		if gotAuth != "token tok" {
			t.Errorf("Authorization = %q", gotAuth)
		}
		w.Write([]byte{0x00, 0xFF, 0x42, 0x0A})
	}))
	defer ts.Close()

	data, err := newTestClient(ts).Download(ts.URL + "/attachments/1/forge.bin")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/attachments/1/forge.bin" {
		t.Errorf("path = %q", gotPath)
	}
	// Binary fetch sends no Accept header; the body survives byte for byte.
	if gotAccept != "" {
		t.Errorf("Accept = %q, want none", gotAccept)
	}
	want := []byte{0x00, 0xFF, 0x42, 0x0A}
	if string(data) != string(want) {
		t.Errorf("body = %v, want %v", data, want)
	}
}

func TestDownloadRejectsOffHostBeforeRequest(t *testing.T) {
	var evilHits int
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilHits++
	}))
	defer evil.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("base server must not be contacted for an off-host asset")
	}))
	defer ts.Close()

	// The asset points at a real second listener on another host:port, so a
	// leaked request would be observable; the check must fire first.
	_, err := newTestClient(ts).Download(evil.URL + "/steal-token")
	if err == nil {
		t.Fatal("off-host download: want error, got nil")
	}
	if evilHits != 0 {
		t.Errorf("off-host server saw %d requests; token must never reach it", evilHits)
	}
	if !strings.Contains(err.Error(), "refusing to send credentials") {
		t.Errorf("err = %v, want credential-refusal message", err)
	}
}

func TestDownloadRejectsSchemeMismatch(t *testing.T) {
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer ts.Close()

	// Same host:port as the base, but https vs the base's http.
	assetURL := "https" + strings.TrimPrefix(ts.URL, "http") + "/asset"
	_, err := newTestClient(ts).Download(assetURL)
	if err == nil {
		t.Fatal("scheme-mismatched download: want error, got nil")
	}
	if hits != 0 {
		t.Errorf("server saw %d requests; check must fire before the request", hits)
	}
}

func TestDownloadSurfacesAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"asset gone"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).Download(ts.URL + "/attachments/1/gone")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusNotFound || apiErr.Message != "asset gone" {
		t.Fatalf("err = %v (%T), want *APIError{404 asset gone}", err, err)
	}
}

func TestDownloadNetworkFailureIsTagged(t *testing.T) {
	// 127.0.0.1:1 is the API base and the asset host, so the host check passes
	// and the request fails at the transport layer.
	client := NewClient("http://127.0.0.1:1", "tok", 0, nil)
	_, err := client.Download("http://127.0.0.1:1/asset")
	if err == nil || !IsNetwork(err) {
		t.Fatalf("err = %v (%T), want tagged network error", err, err)
	}
}
