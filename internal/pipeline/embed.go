package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/arbhalerao/picseek/internal/classify"
	"github.com/arbhalerao/picseek/internal/embed"
	"github.com/arbhalerao/picseek/internal/store"
)

const (
	embedBatch    = 16
	classifyBatch = 64
	maxChunks     = 8
	chunkWords    = 60
	yieldPoll     = 2 * time.Second
	embedTimeout  = 60 * time.Second
)

type EmbedStage struct {
	store      *store.Store
	client     *embed.Client
	classifier atomic.Pointer[classify.Classifier]
	Meter      *Meter
	log        *slog.Logger
}

func NewEmbedStage(st *store.Store, client *embed.Client, log *slog.Logger) *EmbedStage {
	return &EmbedStage{store: st, client: client, Meter: &Meter{}, log: log}
}

func (s *EmbedStage) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	loops := []struct {
		kind   string
		batch  int
		handle func(context.Context, []store.Job) error
	}{
		{store.KindEmbedText, embedBatch, s.embedTexts},
		{store.KindEmbedImage, embedBatch, s.embedImages},
		{store.KindClassify, classifyBatch, s.classify},
	}
	go s.prepare(ctx)
	for _, l := range loops {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.loop(ctx, l.kind, l.batch, l.handle)
		}()
	}
	wg.Wait()
	return nil
}

func (s *EmbedStage) prepare(ctx context.Context) {
	for ctx.Err() == nil {
		if !s.client.Ready() {
			sleep(ctx, time.Second)
			continue
		}
		versions, err := s.client.Versions(ctx)
		if err != nil {
			sleep(ctx, versionRetry)
			continue
		}
		classifier, err := classify.New(ctx, s.client)
		if err != nil {
			s.log.Warn("load classifier prompts", "err", err)
			sleep(ctx, versionRetry)
			continue
		}
		s.classifier.Store(classifier)

		for _, step := range []struct {
			kind, version, after string
		}{
			{store.KindEmbedText, versions.Text, store.KindOCR},
			{store.KindEmbedImage, versions.ClipImage, ""},
			{store.KindClassify, classifier.Version, store.KindEmbedImage},
		} {
			missing, err := s.store.EnqueueMissing(ctx, step.kind, step.after)
			if err != nil {
				s.log.Warn("enqueue missing", "kind", step.kind, "err", err)
				continue
			}
			stale, err := s.store.EnqueueStale(ctx, step.kind, step.version)
			if err != nil {
				s.log.Warn("enqueue stale", "kind", step.kind, "err", err)
				continue
			}
			if missing+stale > 0 {
				s.log.Info("queued enrichment", "kind", step.kind, "version", step.version, "missing", missing, "stale", stale)
			}
		}
		return
	}
}

func (s *EmbedStage) loop(ctx context.Context, kind string, batch int, handle func(context.Context, []store.Job) error) {
	for ctx.Err() == nil {
		if kind == store.KindClassify && s.classifier.Load() == nil || kind != store.KindClassify && !s.client.Ready() {
			sleep(ctx, time.Second)
			continue
		}
		if pending, err := s.store.PendingJobs(ctx, store.KindOCR); err != nil || pending > 0 {
			sleep(ctx, yieldPoll)
			continue
		}
		jobs, err := s.store.ClaimJobs(ctx, kind, batch, lease)
		if err != nil || len(jobs) == 0 {
			sleep(ctx, idlePoll)
			continue
		}
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), embedTimeout)
		err = handle(callCtx, jobs)
		cancel()
		if err != nil {
			s.handleError(kind, jobs, err)
			continue
		}
		s.client.Breaker().Success()
	}
}

func (s *EmbedStage) embedTexts(ctx context.Context, jobs []store.Job) error {
	ids := make([]int64, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ImageID
	}
	texts, err := s.store.Texts(ctx, ids)
	if err != nil {
		return err
	}
	var flat []string
	spans := make([][2]int, len(jobs))
	for i, j := range jobs {
		chunks := Chunks(texts[j.ImageID])
		spans[i] = [2]int{len(flat), len(flat) + len(chunks)}
		flat = append(flat, chunks...)
	}
	var vectors [][]float32
	version := ""
	if len(flat) > 0 {
		if vectors, version, err = s.client.EmbedTexts(ctx, flat); err != nil {
			return err
		}
	} else if v, err := s.client.Versions(ctx); err == nil {
		version = v.Text
	} else {
		return err
	}
	results := make([]store.TextEmbedding, len(jobs))
	for i, j := range jobs {
		results[i] = store.TextEmbedding{JobID: j.ID, ImageID: j.ImageID, Chunks: vectors[spans[i][0]:spans[i][1]], Version: version}
	}
	if err := s.store.CompleteTextEmbeddings(context.Background(), results); err != nil {
		return err
	}
	s.Meter.Add(len(results))
	return nil
}

func (s *EmbedStage) embedImages(ctx context.Context, jobs []store.Job) error {
	var live []store.Job
	var dropped []int64
	for _, j := range jobs {
		if j.Path == "" {
			dropped = append(dropped, j.ID)
			continue
		}
		live = append(live, j)
	}
	if len(dropped) > 0 {
		if err := s.store.DropJobs(context.Background(), dropped); err != nil {
			return err
		}
	}
	if len(live) == 0 {
		return nil
	}
	paths := make([]string, len(live))
	for i, j := range live {
		paths[i] = j.Path
	}
	vectors, version, err := s.client.EmbedImages(ctx, paths)
	if err != nil {
		return err
	}
	var results []store.ImageEmbedding
	var failed []store.JobFailure
	for i, j := range live {
		if len(vectors[i]) == 0 {
			failed = append(failed, store.JobFailure{JobID: j.ID, Err: "image could not be read", Permanent: true})
			continue
		}
		results = append(results, store.ImageEmbedding{JobID: j.ID, ImageID: j.ImageID, Vector: vectors[i], Version: version})
	}
	if err := s.store.FailJobs(context.Background(), failed); err != nil {
		return err
	}
	if err := s.store.CompleteImageEmbeddings(context.Background(), results); err != nil {
		return err
	}
	s.Meter.Add(len(results))
	return nil
}

func (s *EmbedStage) classify(ctx context.Context, jobs []store.Job) error {
	classifier := s.classifier.Load()
	byImage := make(map[int64]int64, len(jobs))
	ids := make([]int64, len(jobs))
	for i, j := range jobs {
		byImage[j.ImageID] = j.ID
		ids[i] = j.ImageID
	}
	inputs, err := s.store.ClassifyInputs(ctx, ids)
	if err != nil {
		return err
	}
	types := make([]store.ImageType, len(inputs))
	seen := make(map[int64]bool, len(inputs))
	for i, in := range inputs {
		types[i] = classifier.Classify(in)
		seen[in.ImageID] = true
	}
	var dropped []int64
	for _, j := range jobs {
		if !seen[j.ImageID] {
			dropped = append(dropped, j.ID)
		}
	}
	if err := s.store.DropJobs(context.Background(), dropped); err != nil {
		return err
	}
	return s.store.CompleteClassification(context.Background(), byImage, types, classifier.Version)
}

func (s *EmbedStage) handleError(kind string, jobs []store.Job, err error) {
	ctx := context.Background()
	code := status.Code(err)
	if code == codes.Unavailable {
		s.client.MarkUnhealthy()
		s.client.Breaker().Failure()
		ids := make([]int64, len(jobs))
		for i, j := range jobs {
			ids[i] = j.ID
		}
		if err := s.store.ReleaseJobs(ctx, ids); err != nil {
			s.log.Error("release jobs", "err", err)
		}
		return
	}
	s.client.Breaker().Failure()
	s.log.Warn("enrichment failed", "kind", kind, "jobs", len(jobs), "code", code.String(), "err", err)
	failures := make([]store.JobFailure, len(jobs))
	for i, j := range jobs {
		f := store.JobFailure{JobID: j.ID, Err: err.Error()}
		if j.Attempts >= maxAttempts || code == codes.InvalidArgument {
			f.Permanent = true
		} else {
			f.RetryAt = time.Now().Add(jitter(backoff[j.Attempts-1]))
		}
		failures[i] = f
	}
	if err := s.store.FailJobs(ctx, failures); err != nil {
		s.log.Error("record failures", "err", err)
	}
}

func Chunks(text string) []string {
	var chunks []string
	var current []string
	words := 0
	for _, line := range strings.Split(text, "\n") {
		n := len(strings.Fields(line))
		if n == 0 {
			continue
		}
		if words > 0 && words+n > chunkWords {
			chunks = append(chunks, strings.Join(current, "\n"))
			current, words = nil, 0
			if len(chunks) == maxChunks {
				return chunks
			}
		}
		current = append(current, line)
		words += n
	}
	if words > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}
	return chunks
}
