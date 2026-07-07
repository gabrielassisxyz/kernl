package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SourceKindAuto   = "auto"
	SourceKindURL    = "url"
	SourceKindGitHub = "github_repo"
)

var ErrUnsafeSourceURL = errors.New("unsafe source URL")

// allowedContentTypePrefixes restricts ingest to textual formats. Binary
// content is rejected even if it happens to be valid UTF-8, because the
// downstream pipeline expects markdown-compatible text.
var allowedContentTypePrefixes = []string{
	"text/",
	"application/json",
	"application/xml",
	"application/xhtml+xml",
	"application/rss+xml",
	"application/atom+xml",
}

// blockedContentTypePrefixes catches archives, executables, and media even if
// a URL tries to smuggle them past the text allow-list.
var blockedContentTypePrefixes = []string{
	"application/octet-stream",
	"application/zip",
	"application/gzip",
	"application/x-gzip",
	"application/x-tar",
	"application/x-executable",
	"application/x-sharedlib",
	"application/pdf",
	"image/",
	"audio/",
	"video/",
	"font/",
}

type SourceDocument struct {
	Kind    string
	URL     string
	Title   string
	Content string
}

func (d SourceDocument) Markdown() string {
	var b strings.Builder
	title := strings.TrimSpace(d.Title)
	if title == "" {
		title = d.URL
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "Source: %s\n", d.URL)
	if d.Kind != "" {
		fmt.Fprintf(&b, "Kind: %s\n", d.Kind)
	}
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(d.Content))
	b.WriteString("\n")
	return b.String()
}

type SourceFetcher struct {
	Client *http.Client
}

func NewSourceFetcher(client *http.Client) SourceFetcher {
	if client == nil {
		client = defaultSourceHTTPClient()
	}
	return SourceFetcher{Client: client}
}

// defaultSourceHTTPClient returns a conservative client for fetching external
// ingest sources. Redirects are followed, but every hop is re-validated to
// prevent SSRF via open redirectors.
func defaultSourceHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: checkSourceRedirect,
	}
}

func checkSourceRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if _, err := validateSourceURL(req.URL.String()); err != nil {
		return fmt.Errorf("redirect target rejected: %w", err)
	}
	return nil
}

func (f SourceFetcher) Fetch(ctx context.Context, rawURL, kind string, maxBytes int64) (SourceDocument, error) {
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	parsed, err := validateSourceURL(rawURL)
	if err != nil {
		return SourceDocument{}, err
	}
	kind = strings.TrimSpace(kind)
	if kind == "" || kind == SourceKindAuto {
		if _, _, ok := parseGitHubRepo(parsed); ok {
			kind = SourceKindGitHub
		} else {
			kind = SourceKindURL
		}
	}
	switch kind {
	case SourceKindURL:
		return f.fetchURL(ctx, parsed, maxBytes)
	case SourceKindGitHub:
		return f.fetchGitHubRepo(ctx, parsed, maxBytes)
	default:
		return SourceDocument{}, fmt.Errorf("unsupported source kind %q", kind)
	}
}

func (f SourceFetcher) fetchURL(ctx context.Context, parsed *url.URL, maxBytes int64) (SourceDocument, error) {
	body, contentType, err := f.fetchText(ctx, parsed.String(), maxBytes)
	if err != nil {
		return SourceDocument{}, err
	}
	title := parsed.Hostname()
	content := body
	if strings.Contains(strings.ToLower(contentType), "html") || looksLikeHTML(body) {
		title = htmlTitle(body, title)
		content = htmlToText(body)
	}
	return SourceDocument{
		Kind:    SourceKindURL,
		URL:     parsed.String(),
		Title:   title,
		Content: content,
	}, nil
}

func (f SourceFetcher) fetchGitHubRepo(ctx context.Context, parsed *url.URL, maxBytes int64) (SourceDocument, error) {
	owner, repo, ok := parseGitHubRepo(parsed)
	if !ok {
		return SourceDocument{}, fmt.Errorf("github_repo source requires a github.com/{owner}/{repo} URL")
	}
	paths := []string{"README.md", "README", "AGENTS.md", "CLAUDE.md", "docs/README.md", "docs/architecture.md"}
	var b strings.Builder
	remaining := maxBytes
	for _, path := range paths {
		if remaining <= 0 {
			break
		}
		raw := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s", owner, repo, path)
		content, _, err := f.fetchText(ctx, raw, remaining)
		if err != nil {
			continue
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", path, content)
		remaining -= int64(len(content))
	}
	if strings.TrimSpace(b.String()) == "" {
		return SourceDocument{}, fmt.Errorf("no readable repository docs found for %s/%s", owner, repo)
	}
	return SourceDocument{
		Kind:    SourceKindGitHub,
		URL:     parsed.String(),
		Title:   owner + "/" + repo,
		Content: b.String(),
	}, nil
}

func (f SourceFetcher) fetchText(ctx context.Context, rawURL string, maxBytes int64) (string, string, error) {
	if _, err := validateSourceURL(rawURL); err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("create source request: %w", err)
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", fmt.Errorf("source not found: %s", rawURL)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("source returned status %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !isAllowedContentType(contentType) {
		return "", "", fmt.Errorf("unsupported content type %q", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", "", fmt.Errorf("read source: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return "", "", fmt.Errorf("source content is too large")
	}
	if !utf8.Valid(body) {
		return "", "", fmt.Errorf("source content is not valid UTF-8 text")
	}
	return string(body), resp.Header.Get("Content-Type"), nil
}

func validateSourceURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse source URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: only http and https URLs are supported", ErrUnsafeSourceURL)
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrUnsafeSourceURL)
	}
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		return nil, fmt.Errorf("%w: localhost is not allowed", ErrUnsafeSourceURL)
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicIP(ip) {
		return nil, fmt.Errorf("%w: private IP targets are not allowed", ErrUnsafeSourceURL)
	}
	return parsed, nil
}

func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

func isAllowedContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	for _, prefix := range blockedContentTypePrefixes {
		if strings.HasPrefix(ct, prefix) {
			return false
		}
	}
	for _, prefix := range allowedContentTypePrefixes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	// Empty or missing content-type is allowed; we fall back to sniffing
	// below. Explicitly unknown types are rejected.
	return ct == ""
}

func parseGitHubRepo(parsed *url.URL) (string, string, bool) {
	if strings.ToLower(parsed.Hostname()) != "github.com" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	return parts[0], repo, true
}

var (
	titleRE     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	brRE        = regexp.MustCompile(`(?i)<br\s*/?>`)
	blockEndRE  = regexp.MustCompile(`(?i)</(p|div|section|article|li|h[1-6])>`)
	stripTagsRE = regexp.MustCompile(`(?s)<[^>]+>`)
	blockTags   = []string{"script", "style", "nav", "footer", "iframe", "noscript"}
	blockTagREs = makeBlockTagREs(blockTags)
)

func makeBlockTagREs(tags []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(tags))
	for i, tag := range tags {
		out[i] = regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
	}
	return out
}

func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<body") || strings.Contains(lower, "<article")
}

func htmlTitle(raw, fallback string) string {
	m := titleRE.FindStringSubmatch(raw)
	if len(m) < 2 {
		return fallback
	}
	title := strings.TrimSpace(collapseSpace(stripTags(m[1])))
	if title == "" {
		return fallback
	}
	return title
}

func htmlToText(raw string) string {
	s := raw
	for _, re := range blockTagREs {
		s = re.ReplaceAllString(s, " ")
	}
	s = brRE.ReplaceAllString(s, "\n")
	s = blockEndRE.ReplaceAllString(s, "\n")
	s = stripTags(s)
	return collapseSpace(s)
}

func stripTags(s string) string {
	return stripTagsRE.ReplaceAllString(s, " ")
}

func collapseSpace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		out = append(out, strings.Join(fields, " "))
	}
	return strings.Join(out, "\n")
}

type StaticSourceFetcher struct {
	Document SourceDocument
	Err      error
}

func (f StaticSourceFetcher) Fetch(ctx context.Context, rawURL, kind string, maxBytes int64) (SourceDocument, error) {
	if f.Err != nil {
		return SourceDocument{}, f.Err
	}
	doc := f.Document
	if doc.URL == "" {
		doc.URL = rawURL
	}
	if doc.Kind == "" {
		doc.Kind = kind
	}
	return doc, nil
}
