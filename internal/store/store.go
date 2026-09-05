package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	write *sql.DB
	read  *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	params := "_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=on"
	write, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?%s&_txlock=immediate", path, params))
	if err != nil {
		return nil, err
	}
	write.SetMaxOpenConns(1)

	if err := migrate(ctx, write); err != nil {
		write.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}

	read, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?%s&_query_only=true", path, params))
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(runtime.NumCPU())

	return &Store{write: write, read: read}, nil
}

func (s *Store) Close() error {
	return errors.Join(s.read.Close(), s.write.Close())
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".sql")
		var exists int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := migrations.ReadFile(file)
		if err != nil {
			return err
		}
		err = inTx(ctx, db, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("%s: %w", version, err)
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	return inTx(ctx, s.write, fn)
}
