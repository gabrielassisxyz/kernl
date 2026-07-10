package ingest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSourceFetcherRejectsUnsafeTargets(t *testing.T) {
	fetcher := NewSourceFetcher(staticHTTPClient(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unsafe target should not be fetched: %s", req.URL.String())
		return nil, nil
	}))
	for _, rawURL := range []string{
		"http://localhost:8080",
		"http://127.0.0.1/status",
		"http://10.0.0.1/status",
		"file:///tmp/secret",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := fetcher.Fetch(context.Background(), rawURL, SourceKindURL, 1024); err == nil {
				t.Fatalf("expected %s to be rejected", rawURL)
			}
		})
	}
}

func TestSourceFetcherRejectsUnsafeRedirect(t *testing.T) {
	var redirectSeen bool
	fetcher := NewSourceFetcher(staticHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "example.com" {
			return redirectResponse("http://127.0.0.1/secret"), nil
		}
		redirectSeen = true
		return textResponse(200, "text/plain", "should not reach here"), nil
	}))
	_, err := fetcher.Fetch(context.Background(), "https://example.com/article", SourceKindURL, 4096)
	if err == nil {
		t.Fatal("expected redirect to private IP to be rejected")
	}
	if redirectSeen {
		t.Fatalf("CheckRedirect allowed the redirect target to be fetched")
	}
}

func TestRejectPrivateDialTargetBlocksResolvedPrivateIPs(t *testing.T) {
	// rejectPrivateDialTarget runs after DNS resolution, so it protects
	// against DNS-based SSRF: a hostname that resolves to a private,
	// loopback, or link-local address is rejected here even though
	// validateSourceURL's textual host check never resolves DNS and would
	// let such a hostname through unnoticed.
	privateAddrs := []string{
		"127.0.0.1:80",
		"10.0.0.1:443",
		"169.254.169.254:80", // cloud metadata endpoint
		"[::1]:80",
	}
	for _, addr := range privateAddrs {
		if err := rejectPrivateDialTarget("tcp", addr, nil); err == nil {
			t.Fatalf("expected resolved address %s to be rejected", addr)
		}
	}
	if err := rejectPrivateDialTarget("tcp", "93.184.216.34:443", nil); err != nil {
		t.Fatalf("expected public resolved address to be allowed, got %v", err)
	}
}

func TestNewSourceFetcherProductionClientRejectsLoopbackDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// This is the exact construction used in production (internal/api/ingest.go
	// calls ingest.NewSourceFetcher(nil)). Its Client must refuse to dial the
	// loopback server even though nothing here goes through validateSourceURL:
	// this proves the dial-level guard, not the textual check, is what closes
	// the DNS-rebinding gap.
	secure := NewSourceFetcher(nil)
	if _, err := secure.Client.Get(srv.URL); err == nil {
		t.Fatal("expected the default (secure) fetcher client to reject a loopback dial target")
	}

	// AllowPrivateHostsForTesting is the opt-in escape hatch tests use to
	// reach loopback servers; production must never pass this option.
	testMode := NewSourceFetcher(nil, AllowPrivateHostsForTesting())
	resp, err := testMode.Client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected AllowPrivateHostsForTesting client to reach the loopback server: %v", err)
	}
	resp.Body.Close()
}

func TestSourceFetcherRejectsBinaryContentType(t *testing.T) {
	fetcher := NewSourceFetcher(staticHTTPClient(func(req *http.Request) (*http.Response, error) {
		return textResponse(200, "application/zip", "PK"), nil
	}))
	_, err := fetcher.Fetch(context.Background(), "https://example.com/file.zip", SourceKindURL, 4096)
	if err == nil {
		t.Fatal("expected binary content type to be rejected")
	}
}

func TestSourceFetcherFetchesHTMLAsText(t *testing.T) {
	fetcher := NewSourceFetcher(staticHTTPClient(func(req *http.Request) (*http.Response, error) {
		return textResponse(200, "text/html", `<html><head><title>Article</title></head><body><article><h1>Hello</h1><p>Readable text.</p></article><script>bad()</script></body></html>`), nil
	}))

	doc, err := fetcher.Fetch(context.Background(), "https://example.com/article", SourceKindURL, 4096)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.Title != "Article" {
		t.Fatalf("Title = %q, want Article", doc.Title)
	}
	if !strings.Contains(doc.Content, "Readable text.") {
		t.Fatalf("expected readable content, got %q", doc.Content)
	}
	if strings.Contains(doc.Content, "bad()") {
		t.Fatalf("script content leaked into extracted text: %q", doc.Content)
	}
}

func TestSourceFetcherFetchesGitHubRepoDocs(t *testing.T) {
	fetcher := NewSourceFetcher(staticHTTPClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/owner/repo/HEAD/README.md":
			return textResponse(200, "text/plain", "# Repo\n\nPrimary documentation."), nil
		case "/owner/repo/HEAD/AGENTS.md":
			return textResponse(200, "text/plain", "# Agent Notes\n\nUse the memory tool."), nil
		default:
			return textResponse(404, "text/plain", "not found"), nil
		}
	}))

	doc, err := fetcher.Fetch(context.Background(), "https://github.com/owner/repo", SourceKindGitHub, 8192)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.Kind != SourceKindGitHub {
		t.Fatalf("Kind = %q, want %q", doc.Kind, SourceKindGitHub)
	}
	if !strings.Contains(doc.Content, "## README.md") || !strings.Contains(doc.Content, "Primary documentation.") {
		t.Fatalf("README content missing from repo document: %q", doc.Content)
	}
	if !strings.Contains(doc.Content, "## AGENTS.md") || !strings.Contains(doc.Content, "Use the memory tool.") {
		t.Fatalf("AGENTS content missing from repo document: %q", doc.Content)
	}
}

func staticHTTPClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	return sourceClient(fn)
}

func sourceClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	client := &http.Client{Transport: roundTripFunc(fn)}
	client.CheckRedirect = checkSourceRedirect
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func textResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func redirectResponse(location string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{location}},
		Body:       io.NopCloser(strings.NewReader("")),
	}
}
