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
)

// ImportPocket parses a Pocket export HTML file and creates bookmarks in the graph.
func ImportPocket(ctx context.Context, tx *graph.WriteTx, r io.Reader, author nodes.Author) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read pocket export: %w", err)
	}
	htmlStr := string(data)

	re := regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(htmlStr, -1)

	count := 0
	for _, m := range matches {
		url := m[1]
		title := m[2]

		b := nodes.Bookmark{
			URL:   url,
			Title: title,
		}
		_, err := nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return count, fmt.Errorf("create bookmark for %s: %w", url, err)
		}
		count++
	}

	return count, nil
}

// ImportPinboard parses a Pinboard JSON export file and creates bookmarks in the graph.
func ImportPinboard(ctx context.Context, tx *graph.WriteTx, r io.Reader, author nodes.Author) (int, error) {
	var items []struct {
		Href        string `json:"href"`
		Description string `json:"description"`
		Extended    string `json:"extended"`
		Tags        string `json:"tags"`
	}
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return 0, fmt.Errorf("decode pinboard export: %w", err)
	}

	count := 0
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
		_, err := nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return count, fmt.Errorf("create bookmark for %s: %w", item.Href, err)
		}
		count++
	}

	return count, nil
}
