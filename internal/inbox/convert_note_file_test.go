package inbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/inbox"
)

// Every note of a fan-out must reach disk with its own body.
//
// The regression this guards: the file name took the FIRST 8 hex of the node id,
// which in uuid v7 is timestamp, so every note created in the same stretch of
// time produced the same name and overwrote the previous one. It was invisible
// from the graph - all the nodes were there - and only cost anything on a cold
// start rebuilt from the markdown, by which point the bodies were gone.
func TestNoteFilesOfOneFanOutDoNotOverwriteEachOther(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "três coisas de uma vez")
	bodies := []string{"primeiro corpo", "segundo corpo", "terceiro corpo"}

	if err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "note", Title: "Um", Body: bodies[0]},
			{Target: "note", Title: "Dois", Body: bodies[1]},
			{Target: "note", Title: "Três", Body: bodies[2]},
		},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	entries, err := os.ReadDir(vault)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	if len(files) != len(bodies) {
		t.Fatalf("expected %d markdown files, got %d: %v", len(bodies), len(files), files)
	}

	// Each body survives exactly once: a collision would have left three files'
	// worth of nodes but fewer distinct bodies on disk.
	onDisk := map[string]bool{}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(vault, name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for _, body := range bodies {
			if strings.Contains(string(data), body) {
				onDisk[body] = true
			}
		}
	}
	for _, body := range bodies {
		if !onDisk[body] {
			t.Errorf("body %q reached no file: a later note overwrote it", body)
		}
	}
}
