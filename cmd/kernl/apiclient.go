package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// apiClient is how every GUI-parity verb reaches the backend: it calls the
// same REST routes the web UI calls, against a running `kernl serve`.
//
// The alternative — importing the internal services directly, the way capture
// and bookmark do — was rejected for this surface: it would duplicate each
// handler's validation, transaction and vault side-effects (~40 times), and it
// cannot reach state that only exists inside the server process (the runtime
// auto-classify flag, chat sessions, SSE streams, the ingest service). capture,
// bookmark and plan stay direct on purpose: they are the offline capture path.
type apiClient struct {
	baseURL string
	http    *http.Client
}

// resolveServerURL picks the server address, most explicit source first:
// --server, then KERNL_SERVER, then 127.0.0.1 on --port, then the port in
// kernl.yaml, then 8080. Config is only read when nothing more specific was
// given, so a --server invocation works with no config file present.
func resolveServerURL(configPath, serverFlag string, port int, env func(string) string) (string, error) {
	if serverFlag != "" {
		return normalizeServerURL(serverFlag)
	}
	if fromEnv := env("KERNL_SERVER"); fromEnv != "" {
		return normalizeServerURL(fromEnv)
	}
	if port == 0 {
		cfg, err := loadCLIConfig(configPath)
		if err != nil {
			return "", err
		}
		port = cfg.Server.Port
	}
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), nil
}

// normalizeServerURL accepts what a human types — "localhost:8080",
// "http://box:8080/", "1.2.3.4" — and returns a scheme-qualified base with no
// trailing slash.
func normalizeServerURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", usagef("KERNL DISPATCH FAILURE: --server requires a URL — example: kernl --server http://127.0.0.1:8080 task list")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", usagef("KERNL DISPATCH FAILURE: --server value %q is not a valid URL — example: kernl --server http://127.0.0.1:8080 task list", raw)
	}
	return strings.TrimSuffix(u.Scheme+"://"+u.Host+u.Path, "/"), nil
}

// verbContext carries what a parity verb needs to build its client, so the
// global flags are parsed once in Dispatch instead of by every verb.
type verbContext struct {
	configPath string
	server     string
	port       int
	// out is where the verb writes its result. Injected so a test can drive a
	// verb end-to-end through Dispatch and read what it printed.
	out io.Writer
}

func (v verbContext) stdout() io.Writer {
	if v.out == nil {
		return os.Stdout
	}
	return v.out
}

func (v verbContext) client() (*apiClient, error) {
	return newAPIClient(v.configPath, v.server, v.port, os.Getenv)
}

func newAPIClient(configPath, serverFlag string, port int, env func(string) string) (*apiClient, error) {
	base, err := resolveServerURL(configPath, serverFlag, port, env)
	if err != nil {
		return nil, err
	}
	return &apiClient{baseURL: base, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

// request performs one API call and returns the raw response body. body is
// encoded as JSON when non-nil; a nil body sends no payload.
func (c *apiClient) request(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, wrapLoud("encoding request body", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return nil, wrapLoud("building request", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.unreachable(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, wrapLoud("reading response", err)
	}
	if resp.StatusCode >= 400 {
		return nil, httpStatusError(method, path, resp.StatusCode, raw)
	}
	return raw, nil
}

// unreachable turns a dial failure into the one error that actually tells the
// caller what to do: start the server. Every parity verb needs it up.
func (c *apiClient) unreachable(err error) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot reach the kernl server at %s — Fix: start it with 'kernl serve', or point elsewhere with --server <url> / KERNL_SERVER: %w", c.baseURL, err)
	}
	return wrapLoud(fmt.Sprintf("request to %s failed", c.baseURL), err)
}

// notFoundError marks a 404 so a verb can tell "this does not exist" apart from
// the other 4xx answers. It wraps a usage error, so a verb that does NOT opt in
// keeps the old behaviour: a bad id is still exit 2, which is right — asking
// about a task that isn't there IS a bad invocation. Only the reads where
// absence is a legitimate answer call getOptional.
type notFoundError struct{ err error }

func (n notFoundError) Error() string { return n.err.Error() }
func (n notFoundError) Unwrap() error { return n.err }

// httpStatusError maps HTTP status onto the CLI's exit-code contract: a 4xx is
// something about the invocation (bad id, bad value) and exits 2; a 5xx is the
// backend failing and exits 1.
func httpStatusError(method, path string, status int, raw []byte) error {
	detail := strings.TrimSpace(string(raw))
	if len(detail) > 400 {
		detail = detail[:400] + "…"
	}
	if detail == "" {
		detail = http.StatusText(status)
	}
	if status == http.StatusNotFound {
		return notFoundError{err: usagef("KERNL DISPATCH FAILURE: %s %s rejected with 404: %s", method, path, detail)}
	}
	if status >= 400 && status < 500 {
		return usagef("KERNL DISPATCH FAILURE: %s %s rejected with %d: %s", method, path, status, detail)
	}
	return fmt.Errorf("KERNL DISPATCH FAILURE: %s %s failed with %d: %s", method, path, status, detail)
}

func (c *apiClient) get(ctx context.Context, path string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, path, nil)
}

// getOptional is for reads where the thing legitimately may not exist yet — a
// briefing that has not been generated, a prep note nobody asked for. Those
// routes answer 404, which the default mapping turns into exit 2, telling a
// caller it invoked the command wrong when it merely asked a question whose
// answer is "none yet". Here absence is a value, not an error: found=false,
// and the verb decides how to say it.
func (c *apiClient) getOptional(ctx context.Context, path string) (raw json.RawMessage, found bool, err error) {
	raw, err = c.request(ctx, http.MethodGet, path, nil)
	if err == nil {
		return raw, true, nil
	}
	var missing notFoundError
	if errors.As(err, &missing) {
		return nil, false, nil
	}
	return nil, false, err
}

func (c *apiClient) post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPost, path, body)
}

func (c *apiClient) patch(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPatch, path, body)
}

func (c *apiClient) put(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPut, path, body)
}

func (c *apiClient) delete(ctx context.Context, path string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodDelete, path, nil)
}
