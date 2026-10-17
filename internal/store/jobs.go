package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/arbhalerao/picseek/internal/textnorm"
)

type Job struct {
	ID       int64
	ImageID  int64
	Attempts int
	Path     string
}

type OCRResult struct {
	JobID   int64
	ImageID int64
	Text    string
	Version string
}

type JobFailure struct {
	JobID     int64
	Err       string
	Permanent bool
	RetryAt   time.Time
}

type QueueCounts struct {
	Indexed        int64
	QueuedLive     int64
	QueuedBackfill int64
	QueuedEnrich   int64
	Failed         int64
}

func enqueue(ctx context.Context, tx *sql.Tx, imageID int64, kind string, lane Lane, priority int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO jobs (image_id, kind, lane, priority) VALUES (?, ?, ?, ?)
		 ON CONFLICT (image_id, kind) DO UPDATE SET lane = MIN(lane, excluded.lane)`,
		imageID, kind, int(lane), priority)
	return err
}

func (s *Store) ClaimJobs(ctx context.Context, kind string, n int, lease time.Duration) ([]Job, error) {
	now := time.Now()
	var jobs []Job
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`UPDATE jobs SET lease_until = ?, attempts = attempts + 1
			 WHERE id IN (
			     SELECT id FROM jobs
			     WHERE kind = ? AND failed = 0 AND lease_until < ? AND not_before <= ?
			     ORDER BY lane, priority DESC
			     LIMIT ?)
			 RETURNING id, image_id, attempts`,
			now.Add(lease).Unix(), kind, now.Unix(), now.Unix(), n)
		if err != nil {
			return err
		}
		for rows.Next() {
			var j Job
			if err := rows.Scan(&j.ID, &j.ImageID, &j.Attempts); err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, j)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for i := range jobs {
			err := tx.QueryRowContext(ctx,
				`SELECT path FROM paths WHERE image_id = ? ORDER BY mtime DESC LIMIT 1`,
				jobs[i].ImageID).Scan(&jobs[i].Path)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		return nil
	})
	return jobs, err
}

func (s *Store) CompleteOCR(ctx context.Context, results []OCRResult) error {
	now := time.Now().Unix()
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, r := range results {
			if _, err := tx.ExecContext(ctx, `DELETE FROM image_text WHERE rowid = ?`, r.ImageID); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO image_text (rowid, text, folded, compact) VALUES (?, ?, ?, ?)`,
				r.ImageID, r.Text, textnorm.Fold(r.Text), textnorm.Compact(r.Text))
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO enrichments (image_id, enricher, version, done_at) VALUES (?, ?, ?, ?)
				 ON CONFLICT (image_id, enricher) DO UPDATE SET version = excluded.version, done_at = excluded.done_at`,
				r.ImageID, KindOCR, r.Version, now)
			if err != nil {
				return err
			}
			var lane Lane
			var priority int64
			err = tx.QueryRowContext(ctx, `SELECT lane, priority FROM jobs WHERE id = ?`, r.JobID).Scan(&lane, &priority)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, r.JobID); err != nil {
				return err
			}
			if err := enqueue(ctx, tx, r.ImageID, KindEmbedText, lane, priority); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) FailJobs(ctx context.Context, failures []JobFailure) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, f := range failures {
			failed := 0
			if f.Permanent {
				failed = 1
			}
			_, err := tx.ExecContext(ctx,
				`UPDATE jobs SET failed = ?, last_error = ?, not_before = ?, lease_until = 0 WHERE id = ?`,
				failed, f.Err, f.RetryAt.Unix(), f.JobID)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ReleaseAllLeases(ctx context.Context) (int64, error) {
	res, err := s.write.ExecContext(ctx,
		`UPDATE jobs SET lease_until = 0, attempts = MAX(attempts - 1, 0) WHERE lease_until > 0 AND failed = 0`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ReleaseJobs(ctx context.Context, ids []int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			_, err := tx.ExecContext(ctx,
				`UPDATE jobs SET lease_until = 0, attempts = MAX(attempts - 1, 0) WHERE id = ?`, id)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DropJobs(ctx context.Context, ids []int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) RetryFailed(ctx context.Context) (int64, error) {
	res, err := s.write.ExecContext(ctx,
		`UPDATE jobs SET failed = 0, attempts = 0, not_before = 0, lease_until = 0, last_error = NULL WHERE failed = 1`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) EnqueueStale(ctx context.Context, kind, version string) (int64, error) {
	res, err := s.write.ExecContext(ctx,
		`INSERT INTO jobs (image_id, kind, lane, priority)
		 SELECT i.id, ?, ?, i.taken_at FROM images i
		 LEFT JOIN enrichments e ON e.image_id = i.id AND e.enricher = ?
		 WHERE i.orphaned_at IS NULL AND e.version IS NOT NULL AND e.version != ?
		 ON CONFLICT (image_id, kind) DO NOTHING`,
		kind, int(LaneBackfill), kind, version)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) Counts(ctx context.Context) (QueueCounts, error) {
	var c QueueCounts
	err := s.read.QueryRowContext(ctx,
		`SELECT
		     (SELECT COUNT(*) FROM enrichments e JOIN images i ON i.id = e.image_id
		      WHERE e.enricher = ? AND i.orphaned_at IS NULL),
		     (SELECT COUNT(*) FROM jobs WHERE failed = 0 AND kind = ? AND lane = ?),
		     (SELECT COUNT(*) FROM jobs WHERE failed = 0 AND kind = ? AND lane = ?),
		     (SELECT COUNT(*) FROM jobs WHERE failed = 0 AND kind != ?),
		     (SELECT COUNT(*) FROM jobs WHERE failed = 1)`,
		KindOCR, KindOCR, int(LaneLive), KindOCR, int(LaneBackfill), KindOCR).Scan(&c.Indexed, &c.QueuedLive, &c.QueuedBackfill, &c.QueuedEnrich, &c.Failed)
	return c, err
}
