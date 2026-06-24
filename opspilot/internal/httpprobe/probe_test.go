package httpprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunGeneratesProbeIDAndSendsHeaders(t *testing.T) {
	var gotProbeID string
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProbeID = r.Header.Get(ProbeHeaderName)
		gotUserAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	result, err := Run(context.Background(), Request{URL: server.URL + "/health", IncludeResponse: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProbeID == "" || !strings.HasPrefix(result.ProbeID, "probe-") {
		t.Fatalf("probe_id = %s", result.ProbeID)
	}
	if gotProbeID != result.ProbeID {
		t.Fatalf("header probe id = %s want %s", gotProbeID, result.ProbeID)
	}
	if gotUserAgent != UserAgentPrefix+"/"+result.ProbeID {
		t.Fatalf("user agent = %s", gotUserAgent)
	}
	if result.StatusCode != http.StatusOK || result.BodyPreview != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunRedactsSensitiveHeadersAndTruncatesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "sid=secret")
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	result, err := Run(context.Background(), Request{
		URL:             server.URL,
		Headers:         map[string]string{"Authorization": "Bearer secret"},
		IncludeResponse: true,
		BodyLimitBytes:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestHeaders["Authorization"] != "[redacted]" {
		t.Fatalf("request headers = %#v", result.RequestHeaders)
	}
	if result.ResponseHeaders["Set-Cookie"] != "[redacted]" {
		t.Fatalf("response headers = %#v", result.ResponseHeaders)
	}
	if result.BodyPreview != "abc" || !result.BodyTruncated {
		t.Fatalf("body preview = %q truncated=%t", result.BodyPreview, result.BodyTruncated)
	}
}

func TestRunRejectsNonHTTPURL(t *testing.T) {
	if _, err := Run(context.Background(), Request{URL: "file:///etc/passwd"}); err == nil {
		t.Fatal("expected non-http URL to fail")
	}
}
