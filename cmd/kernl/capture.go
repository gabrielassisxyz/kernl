package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault"
)

func runCapture(configPath string, args []string) error {
	var text string
	if len(args) > 0 {
		text = strings.Join(args, " ")
	} else {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		text = strings.TrimSpace(string(bytes))
	}

	if text == "" {
		return fmt.Errorf("capture text cannot be empty")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	vault.ApplyDefaults(&cfg.Vault)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, err := graph.Open(ctx, graph.Config{Path: cfg.Vault.Root + "/.kernl-graph.db"})
	if err != nil {
		return fmt.Errorf("open graph: %w", err)
	}
	defer g.Close()

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		c := nodes.Capture{
			Body:         text,
			CapturedFrom: "cli",
			Tags:         []string{"pending"},
		}
		_, err := nodes.CreateCapture(ctx, tx, c, nodes.Author{Name: "cli"})
		return err
	})
	if err != nil {
		return fmt.Errorf("create capture: %w", err)
	}

	fmt.Println("Captured successfully.")
	return nil
}
