package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault"
)

func runCapture(configPath string, args []string) error {
	// --json is only recognized as the FIRST argument — anything after that
	// is capture text. A leading "--" is the end-of-flags sentinel: it lets
	// flag-looking text (e.g. the literal strings "--help" or "--json") be
	// captured on purpose.
	var asJSON bool
	if len(args) > 0 && args[0] == "--json" {
		asJSON = true
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	var text string
	if len(args) > 0 {
		text = strings.Join(args, " ")
	} else {
		if stdinIsTerminal() {
			return usagef("KERNL DISPATCH FAILURE: capture got no text and stdin is a terminal — pass text as an argument (kernl capture \"<text>\") or pipe it in. Run: kernl capture --help")
		}
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		text = strings.TrimSpace(string(bytes))
	}

	if text == "" {
		return usagef("KERNL DISPATCH FAILURE: capture text cannot be empty — pass text as an argument or via stdin. Run: kernl capture --help")
	}

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}
	vault.ApplyDefaults(&cfg.Vault)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, err := graph.Open(ctx, graph.Config{Path: cfg.Vault.Root + "/.kernl-graph.db"})
	if err != nil {
		return fmt.Errorf("open graph: %w", err)
	}
	defer g.Close()

	var captureID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		c := nodes.Capture{
			Body:         text,
			CapturedFrom: "cli",
			Tags:         []string{"pending"},
		}
		id, err := nodes.CreateCapture(ctx, tx, c, nodes.Author{Name: "cli"})
		captureID = id
		return err
	})
	if err != nil {
		return wrapLoud("create capture", err)
	}

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"id": captureID, "status": "captured"})
	}
	fmt.Printf("Captured %s.\n", captureID)
	return nil
}
