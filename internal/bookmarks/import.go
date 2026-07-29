package bookmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// companionFor builds the companion note of a freshly imported bookmark. An
// import creates the same kind of node the API does, so it owes the same note:
// without one the bookmark lives in the graph db alone, which is git-ignored, and
// an import of a thousand links would leave nothing in the backed-up markdown.
//
// Labelled by URL to match every other bookmark companion. The description stays
// empty even when the export carried one: the entity's description is what
// SyncDescription keeps in step later, and stamping an imported value here would
// make the first edit look like drift.
func companionFor(ctx context.Context, tx *graph.WriteTx, vaultRoot, bookmarkID, url string) (companion.File, error) {
	return companion.Create(ctx, tx, vaultRoot, bookmarkID, layout.BookmarksFolder, url, "", "bookmark")
}

// ImportPocket parses a Pocket export HTML file and creates bookmarks in the graph.
func ImportPocket(ctx context.Context, tx *graph.WriteTx, vaultRoot string, r io.Reader, author nodes.Author) (int, []companion.File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, nil, fmt.Errorf("read pocket export: %w", err)
	}
	htmlStr := string(data)

	re := regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(htmlStr, -1)

	count := 0
	var companions []companion.File
	for _, m := range matches {
		url := m[1]
		title := m[2]

		b := nodes.Bookmark{
			URL:   url,
			Title: title,
		}
		id, err := nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return count, nil, fmt.Errorf("create bookmark for %s: %w", url, err)
		}
		cf, err := companionFor(ctx, tx, vaultRoot, id, url)
		if err != nil {
			return count, nil, err
		}
		companions = append(companions, cf)
		count++
	}

	return count, companions, nil
}

// ImportPinboard parses a Pinboard JSON export file and creates bookmarks in the graph.
func ImportPinboard(ctx context.Context, tx *graph.WriteTx, vaultRoot string, r io.Reader, author nodes.Author) (int, []companion.File, error) {
	var items []struct {
		Href        string `json:"href"`
		Description string `json:"description"`
		Extended    string `json:"extended"`
		Tags        string `json:"tags"`
	}
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, nil, fmt.Errorf("decode pinboard export: %w", err)
	}

	count := 0
	var companions []companion.File
	for _, item := range items {
		var tags []string
		if item.Tags != "" {
			tags = strings.Fields(item.Tags)
		}
		b := nodes.Bookmark{
			URL:         item.Href,
			Title:       item.Description,
			Description: item.Extended,
			Tags:        tags,
		}
		id, err := nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return count, nil, fmt.Errorf("create bookmark for %s: %w", item.Href, err)
		}
		cf, err := companionFor(ctx, tx, vaultRoot, id, item.Href)
		if err != nil {
			return count, nil, err
		}
		companions = append(companions, cf)
		count++
	}

	return count, companions, nil
}
