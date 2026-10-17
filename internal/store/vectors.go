package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
)

const (
	KindEmbedText  = "embed_text"
	KindEmbedImage = "embed_image"
	KindClassify   = "classify"
)

type TextEmbedding struct {
	JobID   int64
	ImageID int64
	Chunks  [][]float32
	Version string
}

type ImageEmbedding struct {
	JobID   int64
	ImageID int64
	Vector  []float32
	Version string
}

type VectorQuery struct {
	Vector    []float32
	Baselines [][]float32
	Folder    string
	After     time.Time
	Before    time.Time
	Type      string
	Keep      func(text string) bool
	Min       float64
	Margin    float64
	Limit     int
}

type ImageType struct {
	ImageID int64
	Type    string
	Score   float64
}

type ClassifyInput struct {
	ImageID int64
	Image   []float32
	Text    [][]float32
}

func (s *Store) PendingJobs(ctx context.Context, kind string) (int64, error) {
	var n int64
	err := s.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE kind = ? AND failed = 0`, kind).Scan(&n)
	return n, err
}

func (s *Store) EnqueueMissing(ctx context.Context, kind, after string) (int64, error) {
	query := `INSERT INTO jobs (image_id, kind, lane, priority)
		SELECT i.id, ?, ?, i.taken_at FROM images i
		WHERE i.orphaned_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM enrichments e WHERE e.image_id = i.id AND e.enricher = ?)`
	args := []any{kind, int(LaneBackfill), kind}
	if after != "" {
		query += ` AND EXISTS (SELECT 1 FROM enrichments e WHERE e.image_id = i.id AND e.enricher = ?)`
		args = append(args, after)
	}
	query += ` ON CONFLICT (image_id, kind) DO NOTHING`
	res, err := s.write.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) Texts(ctx context.Context, ids []int64) (map[int64]string, error) {
	texts := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return texts, nil
	}
	rows, err := s.read.QueryContext(ctx,
		`SELECT rowid, text FROM image_text WHERE rowid IN (`+placeholders(len(ids))+`)`, int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		texts[id] = text
	}
	return texts, rows.Err()
}

func (s *Store) CompleteTextEmbeddings(ctx context.Context, results []TextEmbedding) error {
	now := time.Now().Unix()
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, r := range results {
			if _, err := tx.ExecContext(ctx, `DELETE FROM text_vectors WHERE image_id = ?`, r.ImageID); err != nil {
				return err
			}
			for i, v := range r.Chunks {
				_, err := tx.ExecContext(ctx,
					`INSERT INTO text_vectors (image_id, chunk, embedding) VALUES (?, ?, ?)`, r.ImageID, i, encode(v))
				if err != nil {
					return err
				}
			}
			if err := finish(ctx, tx, r.JobID, r.ImageID, KindEmbedText, r.Version, now); err != nil {
				return err
			}
			if err := enqueueClassifyIfReady(ctx, tx, r.ImageID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) CompleteImageEmbeddings(ctx context.Context, results []ImageEmbedding) error {
	now := time.Now().Unix()
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, r := range results {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO image_vectors (image_id, embedding) VALUES (?, ?)
				 ON CONFLICT (image_id) DO UPDATE SET embedding = excluded.embedding`, r.ImageID, encode(r.Vector))
			if err != nil {
				return err
			}
			if err := finish(ctx, tx, r.JobID, r.ImageID, KindEmbedImage, r.Version, now); err != nil {
				return err
			}
			if err := enqueueClassifyIfReady(ctx, tx, r.ImageID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ClassifyInputs(ctx context.Context, ids []int64) ([]ClassifyInput, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	inputs := make(map[int64]*ClassifyInput, len(ids))
	rows, err := s.read.QueryContext(ctx,
		`SELECT image_id, embedding FROM image_vectors WHERE image_id IN (`+placeholders(len(ids))+`)`, int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			rows.Close()
			return nil, err
		}
		inputs[id] = &ClassifyInput{ImageID: id, Image: decode(blob)}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = s.read.QueryContext(ctx,
		`SELECT image_id, embedding FROM text_vectors WHERE image_id IN (`+placeholders(len(ids))+`) ORDER BY image_id, chunk`, int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		if in, ok := inputs[id]; ok {
			in.Text = append(in.Text, decode(blob))
		}
	}
	out := make([]ClassifyInput, 0, len(inputs))
	for _, id := range ids {
		if in, ok := inputs[id]; ok {
			out = append(out, *in)
		}
	}
	return out, rows.Err()
}

func (s *Store) CompleteClassification(ctx context.Context, jobs map[int64]int64, types []ImageType, version string) error {
	now := time.Now().Unix()
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, t := range types {
			var kind any
			if t.Type != "" {
				kind = t.Type
			}
			if _, err := tx.ExecContext(ctx, `UPDATE images SET type = ?, type_score = ? WHERE id = ?`, kind, t.Score, t.ImageID); err != nil {
				return err
			}
			if err := finish(ctx, tx, jobs[t.ImageID], t.ImageID, KindClassify, version, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) SearchTextVectors(ctx context.Context, q VectorQuery) ([]TextHit, error) {
	return s.searchVectors(ctx, q,
		`SELECT v.image_id, v.embedding, i.taken_at, COALESCE(t.text, '')
		 FROM text_vectors v JOIN images i ON i.id = v.image_id
		 LEFT JOIN image_text t ON t.rowid = v.image_id
		 WHERE i.orphaned_at IS NULL`)
}

func (s *Store) SearchImageVectors(ctx context.Context, q VectorQuery) ([]TextHit, error) {
	return s.searchVectors(ctx, q,
		`SELECT v.image_id, v.embedding, i.taken_at, COALESCE(t.text, '')
		 FROM image_vectors v JOIN images i ON i.id = v.image_id
		 LEFT JOIN image_text t ON t.rowid = v.image_id
		 WHERE i.orphaned_at IS NULL`)
}

func (s *Store) searchVectors(ctx context.Context, q VectorQuery, base string) ([]TextHit, error) {
	var b strings.Builder
	b.WriteString(base)
	var args []any
	if !q.After.IsZero() {
		b.WriteString(` AND i.taken_at >= ?`)
		args = append(args, q.After.Unix())
	}
	if !q.Before.IsZero() {
		b.WriteString(` AND i.taken_at < ?`)
		args = append(args, q.Before.Unix())
	}
	if q.Type != "" {
		b.WriteString(` AND i.type = ?`)
		args = append(args, q.Type)
	}
	if q.Folder != "" {
		b.WriteString(` AND EXISTS (SELECT 1 FROM paths p WHERE p.image_id = i.id AND p.dir LIKE ? ESCAPE '\')`)
		args = append(args, "%"+escapeLike(q.Folder)+"%")
	}
	rows, err := s.read.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	best := make(map[int64]*TextHit)
	var blob []byte
	for rows.Next() {
		var id, takenAt int64
		var text string
		if err := rows.Scan(&id, &blob, &takenAt, &text); err != nil {
			return nil, err
		}
		if q.Keep != nil && !q.Keep(text) {
			continue
		}
		sim := dotBlob(q.Vector, blob)
		if len(q.Baselines) > 0 {
			baseline := math.Inf(-1)
			for _, b := range q.Baselines {
				baseline = math.Max(baseline, dotBlob(b, blob))
			}
			sim -= baseline
		}
		if hit, ok := best[id]; !ok {
			best[id] = &TextHit{ImageID: id, Score: sim, TakenAt: takenAt}
		} else if sim > hit.Score {
			hit.Score = sim
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hits := make([]TextHit, 0, len(best))
	top := math.Inf(-1)
	for _, h := range best {
		top = math.Max(top, h.Score)
	}
	for _, h := range best {
		if h.Score >= q.Min && h.Score >= top-q.Margin {
			hits = append(hits, *h)
		}
	}
	slices.SortFunc(hits, func(a, b TextHit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		default:
			return int(b.TakenAt - a.TakenAt)
		}
	})
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	return hits, nil
}

func finish(ctx context.Context, tx *sql.Tx, jobID, imageID int64, enricher, version string, now int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO enrichments (image_id, enricher, version, done_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (image_id, enricher) DO UPDATE SET version = excluded.version, done_at = excluded.done_at`,
		imageID, enricher, version, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, jobID)
	return err
}

func enqueueClassifyIfReady(ctx context.Context, tx *sql.Tx, imageID int64) error {
	var ready int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM enrichments WHERE image_id = ? AND enricher IN (?, ?)`,
		imageID, KindEmbedText, KindEmbedImage).Scan(&ready)
	if err != nil || ready < 2 {
		return err
	}
	var priority int64
	if err := tx.QueryRowContext(ctx, `SELECT taken_at FROM images WHERE id = ?`, imageID).Scan(&priority); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return enqueue(ctx, tx, imageID, KindClassify, LaneBackfill, priority)
}

func encode(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

func decode(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

func dotBlob(q []float32, b []byte) float64 {
	n := min(len(q), len(b)/4)
	var sum float32
	for i := 0; i < n; i++ {
		sum += q[i] * math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return float64(sum)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
