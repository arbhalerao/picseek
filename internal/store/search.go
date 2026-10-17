package store

import (
	"context"
	"strings"
	"time"
)

type TextQuery struct {
	Match   string
	Folder  string
	After   time.Time
	Before  time.Time
	Type    string
	TypeMin float64
	Keep    func(text string) bool
	Limit   int
}

type TextHit struct {
	ImageID int64
	Score   float64
	TakenAt int64
}

func (s *Store) SearchText(ctx context.Context, q TextQuery) ([]TextHit, error) {
	var b strings.Builder
	var args []any
	if q.Match != "" {
		b.WriteString(`SELECT t.rowid, -bm25(image_text), i.taken_at, t.text
			FROM image_text t JOIN images i ON i.id = t.rowid
			WHERE image_text MATCH ? AND i.orphaned_at IS NULL`)
		args = append(args, q.Match)
	} else {
		b.WriteString(`SELECT i.id, 0, i.taken_at, t.text
			FROM images i JOIN image_text t ON t.rowid = i.id
			WHERE i.orphaned_at IS NULL`)
	}
	if !q.After.IsZero() {
		b.WriteString(` AND i.taken_at >= ?`)
		args = append(args, q.After.Unix())
	}
	if !q.Before.IsZero() {
		b.WriteString(` AND i.taken_at < ?`)
		args = append(args, q.Before.Unix())
	}
	if q.Type != "" {
		b.WriteString(` AND i.type = ? AND i.type_score >= ?`)
		args = append(args, q.Type, q.TypeMin)
	}
	if q.Folder != "" {
		b.WriteString(` AND EXISTS (SELECT 1 FROM paths p WHERE p.image_id = i.id AND p.dir LIKE ? ESCAPE '\')`)
		args = append(args, "%"+escapeLike(q.Folder)+"%")
	}
	if q.Match != "" {
		b.WriteString(` ORDER BY bm25(image_text), i.taken_at DESC`)
	} else {
		b.WriteString(` ORDER BY i.taken_at DESC`)
	}
	if q.Keep == nil {
		b.WriteString(` LIMIT ?`)
		args = append(args, q.Limit)
	}

	rows, err := s.read.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []TextHit
	for rows.Next() && len(hits) < q.Limit {
		var h TextHit
		var text string
		if err := rows.Scan(&h.ImageID, &h.Score, &h.TakenAt, &text); err != nil {
			return nil, err
		}
		if q.Keep != nil && !q.Keep(text) {
			continue
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (s *Store) PathsFor(ctx context.Context, ids []int64, folder string) (map[int64][]string, error) {
	paths := make(map[int64][]string, len(ids))
	if len(ids) == 0 {
		return paths, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	query := `SELECT image_id, path FROM paths WHERE image_id IN (` + placeholders[:len(placeholders)-1] + `)`
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	if folder != "" {
		query += ` AND dir LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(folder)+"%")
	}
	query += ` ORDER BY path`
	rows, err := s.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		paths[id] = append(paths[id], p)
	}
	return paths, rows.Err()
}
