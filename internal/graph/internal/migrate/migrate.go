package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

var ErrDirty = fmt.Errorf("migrate: database is dirty")

type Runner struct {
	db *sql.DB
	fs fs.ReadDirFS
}

type Migration struct {
	Version int
	UpSQL   string
	DownSQL string
}

func New(db *sql.DB, fsys fs.ReadDirFS) (*Runner, error) {
	r := &Runner{db: db, fs: fsys}
	if err := r.ensureSchemaMigrations(); err != nil {
		return nil, fmt.Errorf("migrate: ensure schema_migrations: %w", err)
	}
	return r, nil
}

func (r *Runner) ensureSchemaMigrations() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT NOT NULL,
		dirty BOOL NOT NULL,
		PRIMARY KEY (version)
	)`)
	return err
}

func (r *Runner) Current(ctx context.Context) (version int, dirty bool, err error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, false, nil
	}
	if err := rows.Scan(&version, &dirty); err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}

func (r *Runner) loadMigrations() ([]Migration, error) {
	upFiles := make(map[int]string)
	downFiles := make(map[int]string)

	err := fs.WalkDir(r.fs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".up.sql") {
			v, perr := parseVersion(name)
			if perr != nil {
				return fmt.Errorf("migrate: parse version from %s: %w", name, perr)
			}
			content, rerr := fs.ReadFile(r.fs, path)
			if rerr != nil {
				return fmt.Errorf("migrate: read %s: %w", name, rerr)
			}
			upFiles[v] = string(content)
		}
		if strings.HasSuffix(name, ".down.sql") {
			v, perr := parseVersion(name)
			if perr != nil {
				return fmt.Errorf("migrate: parse version from %s: %w", name, perr)
			}
			content, rerr := fs.ReadFile(r.fs, path)
			if rerr != nil {
				return fmt.Errorf("migrate: read %s: %w", name, rerr)
			}
			downFiles[v] = string(content)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("migrate: walk dir: %w", err)
	}

	var versions []int
	for v := range upFiles {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	var migrations []Migration
	for _, v := range versions {
		down, ok := downFiles[v]
		if !ok {
			return nil, fmt.Errorf("migrate: missing down migration for version %d", v)
		}
		migrations = append(migrations, Migration{Version: v, UpSQL: upFiles[v], DownSQL: down})
	}

	return migrations, nil
}

func parseVersion(name string) (int, error) {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", name)
	}
	return strconv.Atoi(parts[0])
}

func (r *Runner) UpTo(ctx context.Context, target int) error {
	ver, dirty, err := r.Current(ctx)
	if err != nil {
		return fmt.Errorf("migrate: current: %w", err)
	}
	if dirty {
		return ErrDirty
	}

	migrations, err := r.loadMigrations()
	if err != nil {
		return fmt.Errorf("migrate: load: %w", err)
	}

	// Only keep migrations with version <= target
	var filtered []Migration
	for _, m := range migrations {
		if m.Version <= target {
			filtered = append(filtered, m)
		}
	}
	return r.applyUp(ctx, ver, filtered)
}

func (r *Runner) Up(ctx context.Context) error {
	ver, dirty, err := r.Current(ctx)
	if err != nil {
		return fmt.Errorf("migrate: current: %w", err)
	}
	if dirty {
		return ErrDirty
	}

	migrations, err := r.loadMigrations()
	if err != nil {
		return fmt.Errorf("migrate: load: %w", err)
	}

	return r.applyUp(ctx, ver, migrations)
}

func (r *Runner) applyUp(ctx context.Context, current int, migrations []Migration) error {
	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		// Mark dirty first — committed outside the migration transaction so
		// a failed migration leaves the dirty flag intact.
		if _, err := r.db.ExecContext(ctx, `INSERT INTO schema_migrations(version, dirty) VALUES (?, 1) ON CONFLICT(version) DO UPDATE SET dirty=1`, m.Version); err != nil {
			return fmt.Errorf("migrate: mark dirty for version %d: %w", m.Version, err)
		}

		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("migrate: begin tx for version %d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, m.UpSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: up version %d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET dirty=0 WHERE version=?`, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: mark clean for version %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit version %d: %w", m.Version, err)
		}
	}
	return nil
}

func (r *Runner) Down(ctx context.Context) error {
	ver, dirty, err := r.Current(ctx)
	if err != nil {
		return fmt.Errorf("migrate: current: %w", err)
	}
	if dirty {
		return ErrDirty
	}
	if ver == 0 {
		return nil
	}

	migrations, err := r.loadMigrations()
	if err != nil {
		return fmt.Errorf("migrate: load: %w", err)
	}

	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if m.Version != ver {
			continue
		}

		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("migrate: begin tx for down version %d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, m.DownSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: down version %d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=?`, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: delete record for version %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit down version %d: %w", m.Version, err)
		}
		break
	}
	return nil
}
