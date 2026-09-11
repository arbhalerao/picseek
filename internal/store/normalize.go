package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/arbhalerao/picseek/internal/textnorm"
)

const textnormKey = "textnorm_version"

func refreshNormalizedText(ctx context.Context, db *sql.DB) error {
	return inTx(ctx, db, func(tx *sql.Tx) error {
		var current string
		err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, textnormKey).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current == textnorm.Version {
			return nil
		}
		if err := renormalize(ctx, tx); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
			textnormKey, textnorm.Version)
		return err
	})
}

func renormalize(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT rowid, text FROM image_text`)
	if err != nil {
		return err
	}
	type doc struct {
		id   int64
		text string
	}
	var docs []doc
	for rows.Next() {
		var d doc
		if err := rows.Scan(&d.id, &d.text); err != nil {
			rows.Close()
			return err
		}
		docs = append(docs, d)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `UPDATE image_text SET folded = ?, compact = ? WHERE rowid = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, d := range docs {
		if _, err := stmt.ExecContext(ctx, textnorm.Fold(d.text), textnorm.Compact(d.text), d.id); err != nil {
			return err
		}
	}
	return nil
}
