package graph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/gabrielassisxyz/kernl/internal/graph/internal/migrate"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/sqlite"
	"github.com/gabrielassisxyz/kernl/internal/graph/schema"
)

type Config struct {
	Path     string
	InMemory bool
}

type Graph struct {
	pool   *sqlite.Pool
	runner *migrate.Runner
	closed atomic.Bool
}

func Open(ctx context.Context, cfg Config) (*Graph, error) {
	pool, err := sqlite.Open(sqlite.Config{Path: cfg.Path, InMemory: cfg.InMemory})
	if err != nil {
		return nil, fmt.Errorf("graph: open db: %w", err)
	}

	runner, err := migrate.New(pool.Write, schema.FS)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("graph: new migrator: %w", err)
	}

	if err := runner.Up(ctx); err != nil {
		pool.Close()
		if errors.Is(err, migrate.ErrDirty) {
			return nil, fmt.Errorf("graph: migrate: %w", ErrSchemaLocked)
		}
		return nil, fmt.Errorf("graph: migrate: %w", err)
	}

	return &Graph{pool: pool, runner: runner}, nil
}

func (g *Graph) Close() error {
	if g.closed.Swap(true) {
		return nil
	}
	return g.pool.Close()
}

// ReadTx is an unforgeable read-only transaction handle.
type ReadTx struct {
	tx *sql.Tx
}

// WriteTx is an unforgeable read-write transaction handle.
type WriteTx struct {
	tx *sql.Tx
}

func (rtx *ReadTx) QueryRow(query string, args ...any) *sql.Row {
	return rtx.tx.QueryRow(query, args...)
}

func (rtx *ReadTx) Query(query string, args ...any) (*sql.Rows, error) {
	return rtx.tx.Query(query, args...)
}

func (wtx *WriteTx) Exec(query string, args ...any) (sql.Result, error) {
	return wtx.tx.Exec(query, args...)
}

func (wtx *WriteTx) QueryRow(query string, args ...any) *sql.Row {
	return wtx.tx.QueryRow(query, args...)
}

func (wtx *WriteTx) Query(query string, args ...any) (*sql.Rows, error) {
	return wtx.tx.Query(query, args...)
}

func (g *Graph) DoRead(ctx context.Context, fn func(*ReadTx) error) error {
	if g.closed.Load() {
		return fmt.Errorf("graph: closed")
	}
	tx, err := g.pool.Read.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("graph: begin read tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(&ReadTx{tx: tx}); err != nil {
		return err
	}
	return tx.Rollback() // Always rollback reads — they never commit
}

func (g *Graph) DoWrite(ctx context.Context, fn func(*WriteTx) error) error {
	if g.closed.Load() {
		return fmt.Errorf("graph: closed")
	}
	tx, err := g.pool.Write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("graph: begin write tx: %w", err)
	}

	if err := fn(&WriteTx{tx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
