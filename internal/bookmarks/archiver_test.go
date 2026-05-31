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
	defer os.RemoveAll(tmpDir)

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
	defer os.RemoveAll(tmpDir)

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
