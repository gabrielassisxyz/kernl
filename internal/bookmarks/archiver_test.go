package bookmarks

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func newTestClient(fn roundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func TestArchiver_ArchiveBookmark(t *testing.T) {
	client := newTestClient(func(req *http.Request) *http.Response {
		if req.URL.String() == "https://example.com" {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString("<html><body>example</body></html>")),
				Header:     make(http.Header),
			}
		}
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString("not found")),
			Header:     make(http.Header),
		}
	})

	tmpDir, err := os.MkdirTemp("", "archiver-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archiver := NewArchiver(client, tmpDir)

	b := &nodes.Bookmark{
		ID:  "bkm-123",
		URL: "https://example.com",
	}

	res, err := archiver.ArchiveBookmark(context.Background(), b)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res == nil {
		t.Fatal("expected result, got nil")
	}

	if b.ArchivedAt == nil {
		t.Error("expected b.ArchivedAt to be set")
	}

	// Verify HTML was saved
	htmlBytes, err := os.ReadFile(res.HTMLPath)
	if err != nil {
		t.Fatalf("failed to read html: %v", err)
	}
	if string(htmlBytes) != "<html><body>example</body></html>" {
		t.Errorf("unexpected html content: %s", string(htmlBytes))
	}

	// Verify meta was saved
	metaPath := filepath.Join(tmpDir, "bkm-123", "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read meta: %v", err)
	}
	if !bytes.Contains(metaBytes, []byte(`"type": "link"`)) {
		t.Errorf("expected meta to contain type link, got: %s", string(metaBytes))
	}
}

func TestArchiver_ArchiveBookmark_NotFound(t *testing.T) {
	client := newTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString("not found")),
			Header:     make(http.Header),
		}
	})

	tmpDir, err := os.MkdirTemp("", "archiver-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archiver := NewArchiver(client, tmpDir)

	b := &nodes.Bookmark{
		ID:  "bkm-123",
		URL: "https://example.com/notfound",
	}

	_, err = archiver.ArchiveBookmark(context.Background(), b)
	if err == nil {
		t.Fatal("expected error for 404 status, got nil")
	}
}

// TestArchiveBookmarkFillsPlaceholderTitle covers the defect the whole title
// path exists for: a bookmark is created before its page is fetched, so its
// title starts as a stand-in, and the archiver is the only place every surface
// passes through with the HTML in hand.
func TestArchiveBookmarkFillsPlaceholderTitle(t *testing.T) {
	const page = `<html><head><title>The Real Title</title></head><body>text</body></html>`

	cases := []struct {
		name    string
		initial string
		want    string
	}{
		{"legacy api placeholder", "Pending", "The Real Title"},
		{"legacy cli placeholder", "Imported via CLI", "The Real Title"},
		{"url standing in", "https://example.com", "The Real Title"},
		{"empty", "", "The Real Title"},
		{"a title the user supplied is never overwritten", "My Own Name For It", "My Own Name For It"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(func(req *http.Request) *http.Response {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(page)),
					Header:     make(http.Header),
				}
			})
			archiver := NewArchiver(client, t.TempDir())

			b := &nodes.Bookmark{ID: "bkm-1", URL: "https://example.com", Title: tc.initial}
			if _, err := archiver.ArchiveBookmark(context.Background(), b); err != nil {
				t.Fatalf("archive: %v", err)
			}
			if b.Title != tc.want {
				t.Errorf("title = %q, want %q", b.Title, tc.want)
			}
		})
	}
}

// A page that states no title must leave the stand-in alone rather than blank
// the bookmark: a URL is a poor title, an empty one is an unfindable node.
func TestArchiveBookmarkKeepsStandInWhenPageStatesNoTitle(t *testing.T) {
	client := newTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString("<html><body>no title here</body></html>")),
			Header:     make(http.Header),
		}
	})
	archiver := NewArchiver(client, t.TempDir())

	b := &nodes.Bookmark{ID: "bkm-2", URL: "https://example.com", Title: "https://example.com"}
	if _, err := archiver.ArchiveBookmark(context.Background(), b); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if b.Title != "https://example.com" {
		t.Errorf("title = %q, want the URL kept as stand-in", b.Title)
	}
}

// TestArchiveDirIsTheSameForEverySurface pins the one thing the two hardcoded
// paths got wrong: an archive has to land in the same place no matter which
// surface created the bookmark, or a page archived through the inbox is
// unreachable from everything that looks for it.
func TestArchiveDirIsTheSameForEverySurface(t *testing.T) {
	const vault = "/home/u/vault"
	want := filepath.Join(vault, ".kernl", "archives")
	if got := ArchiveDir(vault); got != want {
		t.Errorf("ArchiveDir(%q) = %q, want %q", vault, got, want)
	}
}
