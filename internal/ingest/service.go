package ingest

import (
	"context"
	"fmt"
	"os"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// Service provides active ingest processing functionality.
type Service struct {
	graph     *graph.Graph
	manifest  *ManifestManager
	extractor Extractor
}

// NewService creates a new ingest service.
func NewService(g *graph.Graph, mm *ManifestManager, ex Extractor) *Service {
	return &Service{
		graph:     g,
		manifest:  mm,
		extractor: ex,
	}
}

// ProcessFile reads a file, checks the manifest, uses the extractor if needed,
// and saves actions as IngestReview nodes.
func (s *Service) ProcessFile(ctx context.Context, filePath string, nodeID string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if !s.manifest.NeedsProcessing(filePath, content) {
		return nil
	}

	actions, err := s.extractor.ExtractActions(ctx, string(content))
	if err != nil {
		return fmt.Errorf("extract actions: %w", err)
	}

	for _, act := range actions {
		ir := nodes.IngestReview{
			Title:        act.Title,
			SourceNodeID: nodeID,
			Action:       act.Type,
			Payload:      act.Payload,
			ContentHash:  calculateHash(content),
		}
		err = s.graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := nodes.CreateIngestReview(ctx, tx, ir, nodes.Author{Name: "ingest_service"})
			return err
		})
		if err != nil {
			return fmt.Errorf("create ingest review: %w", err)
		}
	}

	return s.manifest.MarkProcessed(filePath, content)
}
