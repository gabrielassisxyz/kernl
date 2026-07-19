package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveServerURLPrecedence(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	tests := []struct {
		name       string
		serverFlag string
		port       int
		env        map[string]string
		want       string
	}{
		{name: "flag wins over env and port", serverFlag: "http://box:9000", port: 1234,
			env: map[string]string{"KERNL_SERVER": "http://env:1"}, want: "http://box:9000"},
		{name: "env wins over port", port: 1234,
			env: map[string]string{"KERNL_SERVER": "http://env:1"}, want: "http://env:1"},
		{name: "port builds a loopback url", port: 1234, want: "http://127.0.0.1:1234"},
		{name: "bare host gets a scheme", serverFlag: "localhost:8080", want: "http://localhost:8080"},
		{name: "trailing slash is trimmed", serverFlag: "http://box:9000/", want: "http://box:9000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveServerURL("kernl.yaml", tt.serverFlag, tt.port, env(tt.env))
			if err != nil {
				t.Fatalf("resolveServerURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// A --server invocation must not need a config file: the whole point of the
// flag is pointing the CLI at a server from anywhere on the filesystem.
func TestResolveServerURLSkipsConfigWhenServerGiven(t *testing.T) {
	got, err := resolveServerURL("/nonexistent/kernl.yaml", "http://box:9000", 0, func(string) string { return "" })
	if err != nil {
		t.Fatalf("resolveServerURL should not have read the config: %v", err)
	}
	if got != "http://box:9000" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeServerURLRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "   ", "http://"} {
		if _, err := normalizeServerURL(raw); err == nil {
			t.Errorf("normalizeServerURL(%q) accepted an invalid value", raw)
		} else if exitCode(err) != 2 {
			t.Errorf("normalizeServerURL(%q) exit code = %d, want 2 (usage)", raw, exitCode(err))
		}
	}
}

// The exit-code contract the parity verbs rely on: a 4xx is about the
// invocation (exit 2), a 5xx is the backend failing (exit 1).
func TestRequestMapsStatusToExitCode(t *testing.T) {
	tests := []struct {
		status   int
		wantCode int
	}{
		{http.StatusBadRequest, 2},
		{http.StatusNotFound, 2},
		{http.StatusConflict, 2},
		{http.StatusInternalServerError, 1},
		{http.StatusBadGateway, 1},
	}

	for _, tt := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		}))
		c := &apiClient{baseURL: ts.URL, http: ts.Client()}
		_, err := c.get(context.Background(), "/api/tasks")
		ts.Close()

		if err == nil {
			t.Fatalf("status %d: expected an error", tt.status)
		}
		if got := exitCode(err); got != tt.wantCode {
			t.Errorf("status %d: exit code = %d, want %d", tt.status, got, tt.wantCode)
		}
		if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("status %d: error is missing the loud marker: %v", tt.status, err)
		}
	}
}

// The single most likely failure for every parity verb is "the server is not
// running", so that error must name the command that fixes it.
func TestUnreachableServerNamesTheFix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close() // nothing is listening now

	c := &apiClient{baseURL: url, http: &http.Client{}}
	_, err := c.get(context.Background(), "/api/tasks")
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if !strings.Contains(err.Error(), "kernl serve") {
		t.Errorf("unreachable error does not name the fix: %v", err)
	}
	if got := exitCode(err); got != 1 {
		t.Errorf("exit code = %d, want 1 (runtime)", got)
	}
}

func TestRequestSendsJSONBody(t *testing.T) {
	var gotMethod, gotPath, gotType, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotType = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_, _ = w.Write([]byte(`{"id":"t1"}`))
	}))
	defer ts.Close()

	c := &apiClient{baseURL: ts.URL, http: ts.Client()}
	raw, err := c.post(context.Background(), "/api/tasks", map[string]string{"title": "write it"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/tasks" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q", gotType)
	}
	if !strings.Contains(gotBody, `"title":"write it"`) {
		t.Errorf("body = %q", gotBody)
	}
	if string(raw) != `{"id":"t1"}` {
		t.Errorf("response passed through as %q", raw)
	}
}

// A GET must not carry a body or a JSON content type — some handlers branch on
// it, and an empty body is the difference between a read and a malformed write.
func TestGetSendsNoBody(t *testing.T) {
	var hadType bool
	var length int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hadType = r.Header.Get("Content-Type") != ""
		length = r.ContentLength
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	c := &apiClient{baseURL: ts.URL, http: ts.Client()}
	if _, err := c.get(context.Background(), "/api/tasks"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if hadType {
		t.Error("GET sent a Content-Type header")
	}
	if length > 0 {
		t.Errorf("GET sent a %d-byte body", length)
	}
}
