package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/arbhalerao/picseek/internal/ocr"
	"github.com/arbhalerao/picseek/internal/store"
)

const (
	lease          = 60 * time.Second
	callTimeout    = 30 * time.Second
	maxAttempts    = 3
	idlePoll       = 2 * time.Second
	unavailableGap = 1 * time.Second
	commitBatch    = 50
	commitInterval = 250 * time.Millisecond
	versionRetry   = 5 * time.Second
)

var backoff = []time.Duration{5 * time.Second, 30 * time.Second}

type OCRStage struct {
	store *store.Store
	pool  *ocr.Pool
	wake  <-chan struct{}
	Meter *Meter
	log   *slog.Logger
}

type outcome struct {
	job     store.Job
	result  ocr.Result
	failure *store.JobFailure
	drop    bool
	release bool
	gone    string
}

func NewOCRStage(st *store.Store, pool *ocr.Pool, wake <-chan struct{}, log *slog.Logger) *OCRStage {
	return &OCRStage{store: st, pool: pool, wake: wake, Meter: &Meter{}, log: log}
}

func (s *OCRStage) Run(ctx context.Context) error {
	outcomes := make(chan outcome, commitBatch*2)
	var workers sync.WaitGroup
	for _, w := range s.pool.Workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.work(ctx, w, outcomes)
		}()
	}
	go s.reprocessOnVersionChange(ctx)
	go func() {
		workers.Wait()
		close(outcomes)
	}()
	return s.commit(outcomes)
}

func (s *OCRStage) work(ctx context.Context, w *ocr.Worker, outcomes chan<- outcome) {
	for ctx.Err() == nil {
		if !w.Ready() {
			sleep(ctx, unavailableGap)
			continue
		}
		jobs, err := s.store.ClaimJobs(ctx, store.KindOCR, 1, lease)
		if err != nil {
			if ctx.Err() == nil {
				s.log.Warn("claim jobs", "err", err)
			}
			sleep(ctx, idlePoll)
			continue
		}
		if len(jobs) == 0 {
			select {
			case <-ctx.Done():
			case <-s.wake:
			case <-time.After(idlePoll):
			}
			continue
		}
		outcomes <- s.process(ctx, w, jobs[0])
	}
}

func (s *OCRStage) process(ctx context.Context, w *ocr.Worker, job store.Job) outcome {
	if job.Path == "" {
		return outcome{job: job, drop: true}
	}
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), callTimeout)
	defer cancel()
	result, err := w.Recognize(callCtx, job.Path)
	if err == nil {
		w.Breaker().Success()
		return outcome{job: job, result: result}
	}

	code := status.Code(err)
	switch code {
	case codes.InvalidArgument:
		w.Breaker().Success()
		return outcome{job: job, failure: &store.JobFailure{JobID: job.ID, Err: err.Error(), Permanent: true}}
	case codes.NotFound:
		w.Breaker().Success()
		return outcome{job: job, gone: job.Path, failure: &store.JobFailure{JobID: job.ID, Err: err.Error(), RetryAt: time.Now()}}
	case codes.Unavailable:
		w.MarkUnhealthy()
		w.Breaker().Failure()
		s.log.Warn("ocr worker unreachable, job released", "worker", w.Name, "err", err)
		return outcome{job: job, release: true}
	}

	w.Breaker().Failure()
	s.log.Warn("ocr failed", "worker", w.Name, "path", job.Path, "attempt", job.Attempts, "code", code.String(), "err", err)
	if job.Attempts >= maxAttempts {
		return outcome{job: job, failure: &store.JobFailure{JobID: job.ID, Err: err.Error(), Permanent: true}}
	}
	return outcome{job: job, failure: &store.JobFailure{JobID: job.ID, Err: err.Error(), RetryAt: time.Now().Add(jitter(backoff[job.Attempts-1]))}}
}

func (s *OCRStage) commit(outcomes <-chan outcome) error {
	ctx := context.Background()
	var done []store.OCRResult
	var failed []store.JobFailure
	var dropped, released []int64
	var gone []string
	ticker := time.NewTicker(commitInterval)
	defer ticker.Stop()

	flush := func() error {
		if len(gone) > 0 {
			if err := s.store.RemovePaths(ctx, gone); err != nil {
				return err
			}
		}
		if len(done) > 0 {
			if err := s.store.CompleteOCR(ctx, done); err != nil {
				return err
			}
			s.Meter.Add(len(done))
		}
		if len(failed) > 0 {
			if err := s.store.FailJobs(ctx, failed); err != nil {
				return err
			}
		}
		if len(dropped) > 0 {
			if err := s.store.DropJobs(ctx, dropped); err != nil {
				return err
			}
		}
		if len(released) > 0 {
			if err := s.store.ReleaseJobs(ctx, released); err != nil {
				return err
			}
		}
		done, failed, dropped, released, gone = done[:0], failed[:0], dropped[:0], released[:0], gone[:0]
		return nil
	}

	for {
		select {
		case o, ok := <-outcomes:
			if !ok {
				return flush()
			}
			switch {
			case o.drop:
				dropped = append(dropped, o.job.ID)
			case o.release:
				released = append(released, o.job.ID)
			case o.failure != nil:
				failed = append(failed, *o.failure)
				if o.gone != "" {
					gone = append(gone, o.gone)
				}
			default:
				done = append(done, store.OCRResult{JobID: o.job.ID, ImageID: o.job.ImageID, Text: o.result.Text, Version: o.result.Version})
			}
			if len(outcomes) == 0 || len(done)+len(failed)+len(dropped)+len(released) >= commitBatch {
				if err := flush(); err != nil {
					s.log.Error("commit ocr results", "err", err)
				}
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				s.log.Error("commit ocr results", "err", err)
			}
		}
	}
}

func (s *OCRStage) reprocessOnVersionChange(ctx context.Context) {
	for ctx.Err() == nil {
		version, err := s.pool.Version(ctx)
		if err != nil {
			sleep(ctx, versionRetry)
			continue
		}
		n, err := s.store.EnqueueStale(ctx, store.KindOCR, version)
		switch {
		case err != nil && !errors.Is(err, context.Canceled):
			s.log.Warn("enqueue stale ocr results", "err", err)
		case n > 0:
			s.log.Info("ocr engine changed, reprocessing", "version", version, "images", n)
		}
		return
	}
}

func jitter(d time.Duration) time.Duration {
	return d/2 + rand.N(d)
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
