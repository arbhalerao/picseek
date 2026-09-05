package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Lane int

const (
	LaneLive Lane = iota
	LaneBackfill
)

const KindOCR = "ocr"

type Stamp struct {
	Size  int64
	Mtime int64
}

type FileRecord struct {
	Path    string
	Root    string
	Dir     string
	Size    int64
	Mtime   int64
	Hash    uint64
	TakenAt time.Time
	Lane    Lane
}

type UpsertResult struct {
	NewImages int
	Reused    int
}

func (s *Store) AddFolder(ctx context.Context, path string) (bool, error) {
	res, err := s.write.ExecContext(ctx,
		`INSERT INTO folders (path, added_at) VALUES (?, ?) ON CONFLICT (path) DO NOTHING`,
		path, time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) Folders(ctx context.Context) ([]string, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT path FROM folders ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var folders []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		folders = append(folders, p)
	}
	return folders, rows.Err()
}

func (s *Store) ForgetFolder(ctx context.Context, root string) (int64, error) {
	var removed int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM folders WHERE path = ?`, root); err != nil {
			return err
		}
		images, err := imageIDs(ctx, tx, `SELECT DISTINCT image_id FROM paths WHERE root = ?`, root)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM paths WHERE root = ?`, root)
		if err != nil {
			return err
		}
		if removed, err = res.RowsAffected(); err != nil {
			return err
		}
		return markOrphans(ctx, tx, images)
	})
	return removed, err
}

func (s *Store) Stamps(ctx context.Context, root string) (map[string]Stamp, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT path, size, mtime FROM paths WHERE root = ?`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stamps := make(map[string]Stamp)
	for rows.Next() {
		var p string
		var st Stamp
		if err := rows.Scan(&p, &st.Size, &st.Mtime); err != nil {
			return nil, err
		}
		stamps[p] = st
	}
	return stamps, rows.Err()
}

func (s *Store) UpsertFiles(ctx context.Context, files []FileRecord) (UpsertResult, error) {
	var result UpsertResult
	now := time.Now().Unix()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		for _, f := range files {
			hash := int64(f.Hash)
			var imageID int64
			err := tx.QueryRowContext(ctx, `SELECT id FROM images WHERE hash = ?`, hash).Scan(&imageID)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				res, err := tx.ExecContext(ctx,
					`INSERT INTO images (hash, taken_at, created_at) VALUES (?, ?, ?)`,
					hash, f.TakenAt.Unix(), now)
				if err != nil {
					return err
				}
				if imageID, err = res.LastInsertId(); err != nil {
					return err
				}
				if err := enqueue(ctx, tx, imageID, KindOCR, f.Lane, f.TakenAt.Unix()); err != nil {
					return err
				}
				result.NewImages++
			case err != nil:
				return err
			default:
				if _, err := tx.ExecContext(ctx, `UPDATE images SET orphaned_at = NULL WHERE id = ?`, imageID); err != nil {
					return err
				}
				result.Reused++
			}

			var previous sql.NullInt64
			err = tx.QueryRowContext(ctx, `SELECT image_id FROM paths WHERE path = ?`, f.Path).Scan(&previous)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO paths (path, root, dir, image_id, size, mtime) VALUES (?, ?, ?, ?, ?, ?)
				 ON CONFLICT (path) DO UPDATE SET root = excluded.root, dir = excluded.dir,
				 image_id = excluded.image_id, size = excluded.size, mtime = excluded.mtime`,
				f.Path, f.Root, f.Dir, imageID, f.Size, f.Mtime)
			if err != nil {
				return err
			}
			if previous.Valid && previous.Int64 != imageID {
				if err := markOrphans(ctx, tx, []int64{previous.Int64}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) RemovePaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		var images []int64
		for _, p := range paths {
			var id int64
			err := tx.QueryRowContext(ctx, `DELETE FROM paths WHERE path = ? RETURNING image_id`, p).Scan(&id)
			switch {
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return err
			default:
				images = append(images, id)
			}
		}
		return markOrphans(ctx, tx, images)
	})
}

func (s *Store) RemovePathsUnder(ctx context.Context, dir string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		pattern := escapeLike(dir) + "/%"
		images, err := imageIDs(ctx, tx, `SELECT DISTINCT image_id FROM paths WHERE path LIKE ? ESCAPE '\'`, pattern)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM paths WHERE path LIKE ? ESCAPE '\'`, pattern); err != nil {
			return err
		}
		return markOrphans(ctx, tx, images)
	})
}

func (s *Store) CollectGarbage(ctx context.Context, orphanedBefore time.Time) (int64, error) {
	var removed int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		ids, err := imageIDs(ctx, tx, `SELECT id FROM images WHERE orphaned_at IS NOT NULL AND orphaned_at < ?`, orphanedBefore.Unix())
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `DELETE FROM image_text WHERE rowid = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM images WHERE id = ?`, id); err != nil {
				return err
			}
		}
		removed = int64(len(ids))
		return nil
	})
	return removed, err
}

func markOrphans(ctx context.Context, tx *sql.Tx, images []int64) error {
	now := time.Now().Unix()
	for _, id := range images {
		_, err := tx.ExecContext(ctx,
			`UPDATE images SET orphaned_at = ? WHERE id = ? AND orphaned_at IS NULL
			 AND NOT EXISTS (SELECT 1 FROM paths WHERE image_id = ?)`, now, id, id)
		if err != nil {
			return err
		}
	}
	return nil
}

func imageIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func escapeLike(s string) string {
	r := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '%' || c == '_' || c == '\\' {
			r = append(r, '\\')
		}
		r = append(r, c)
	}
	return string(r)
}
