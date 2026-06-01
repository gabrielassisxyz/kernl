package bookmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// Archiver handles the fetching and archiving of web pages.
type Archiver struct {
	client  *http.Client
	dataDir string
}

// NewArchiver creates a new Archiver.
func NewArchiver(client *http.Client, dataDir string) *Archiver {
	if client == nil {
		client = http.DefaultClient
	}
	return &Archiver{
		client:  client,
		dataDir: dataDir,
	}
}

// ArchiveResult contains the outcome of an archiving operation.
type ArchiveResult struct {
	BookmarkID     string
	HTMLPath       string
	ScreenshotPath string
}

// ArchiveBookmark fetches the given URL and archives its raw HTML to disk.
// It also records metadata to allow a future worker to take a screenshot of type=link.
func (a *Archiver) ArchiveBookmark(ctx context.Context, b *nodes.Bookmark) (*ArchiveResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	archiveDir := filepath.Join(a.dataDir, b.ID)
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	htmlPath := filepath.Join(archiveDir, "raw.html")
	if err := os.WriteFile(htmlPath, body, 0644); err != nil {
		return nil, fmt.Errorf("write raw HTML: %w", err)
	}

	// Record metadata for future headless screenshotting
	metaPath := filepath.Join(archiveDir, "meta.json")
	meta := map[string]string{
		"url":  b.URL,
		"type": "link",
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err == nil {
		if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
			return nil, fmt.Errorf("write meta: %w", err)
		}
	} else {
		return nil, fmt.Errorf("marshal meta: %w", err)
	}

	now := time.Now()
	b.ArchivedAt = &now

	return &ArchiveResult{
		BookmarkID:     b.ID,
		HTMLPath:       htmlPath,
		ScreenshotPath: filepath.Join(archiveDir, "screenshot.png"),
	}, nil
}
