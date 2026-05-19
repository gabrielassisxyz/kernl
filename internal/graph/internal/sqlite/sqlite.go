package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Config struct {
	Path     string
	InMemory bool
}

type Pool struct {
	Read  *sql.DB
	Write *sql.DB
}

func Open(cfg Config) (*Pool, error) {
	var dsn string
	if cfg.InMemory {
		dsn = fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", cfg.Path)
	} else {
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=temp_store(2)", cfg.Path)
	}

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open read pool: %w", err)
	}

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		read.Close()
		return nil, fmt.Errorf("sqlite: open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)

	if err := read.Ping(); err != nil {
		read.Close()
		write.Close()
		return nil, fmt.Errorf("sqlite: ping read pool: %w", err)
	}
	if err := write.Ping(); err != nil {
		read.Close()
		write.Close()
		return nil, fmt.Errorf("sqlite: ping write pool: %w", err)
	}

	return &Pool{Read: read, Write: write}, nil
}

func (p *Pool) Close() error {
	wErr := p.Write.Close()
	rErr := p.Read.Close()
	if wErr != nil {
		return wErr
	}
	return rErr
}
