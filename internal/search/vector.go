package search

import (
	"context"
	"strings"
	"time"

	"github.com/arbhalerao/picseek/internal/store"
)

const queryEmbedTimeout = 2 * time.Second

var NullPrompts = []string{
	"a screenshot of a computer screen",
	"a screenshot",
	"a screenshot of a phone screen",
	"a photo",
}

type QueryEmbedder interface {
	Available() bool
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, string, error)
	EmbedClipTexts(ctx context.Context, texts []string) ([][]float32, string, error)
}

type Threshold struct {
	Min    float64
	Margin float64
}

type SemanticRetriever struct {
	store     *store.Store
	embedder  QueryEmbedder
	threshold Threshold
}

func NewSemanticRetriever(st *store.Store, e QueryEmbedder, t Threshold) *SemanticRetriever {
	return &SemanticRetriever{store: st, embedder: e, threshold: t}
}

func (r *SemanticRetriever) Retrieve(ctx context.Context, q Query, limit int) ([]Candidate, error) {
	return vectorRetrieve(ctx, q, limit, r.embedder, r.embedder.EmbedTexts, nil, r.store.SearchTextVectors, r.threshold, MatchMeaning)
}

type VisualRetriever struct {
	store     *store.Store
	embedder  QueryEmbedder
	threshold Threshold
}

func NewVisualRetriever(st *store.Store, e QueryEmbedder, t Threshold) *VisualRetriever {
	return &VisualRetriever{store: st, embedder: e, threshold: t}
}

func (r *VisualRetriever) Retrieve(ctx context.Context, q Query, limit int) ([]Candidate, error) {
	return vectorRetrieve(ctx, q, limit, r.embedder, r.embedder.EmbedClipTexts, NullPrompts, r.store.SearchImageVectors, r.threshold, MatchVisual)
}

func vectorRetrieve(
	ctx context.Context,
	q Query,
	limit int,
	e QueryEmbedder,
	embed func(context.Context, []string) ([][]float32, string, error),
	nullPrompts []string,
	search func(context.Context, store.VectorQuery) ([]store.TextHit, error),
	t Threshold,
	kind MatchKind,
) ([]Candidate, error) {
	text := q.Text()
	if text == "" || !e.Available() {
		return nil, nil
	}
	embedCtx, cancel := context.WithTimeout(ctx, queryEmbedTimeout)
	vectors, _, err := embed(embedCtx, append([]string{text}, nullPrompts...))
	cancel()
	if err != nil || len(vectors) != 1+len(nullPrompts) {
		return nil, nil
	}
	hits, err := search(ctx, store.VectorQuery{
		Vector:    vectors[0],
		Baselines: vectors[1:],
		Folder:    q.Folder,
		After:     q.After,
		Before:    q.Before,
		Type:      q.Type,
		Keep:      excludeFunc(q),
		Min:       t.Min,
		Margin:    t.Margin,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, len(hits))
	for i, h := range hits {
		out[i] = Candidate{ImageID: h.ImageID, Kind: kind, Score: h.Score, TakenAt: h.TakenAt}
	}
	return out, nil
}

func (q Query) Text() string {
	parts := make([]string, 0, len(q.Words)+len(q.Phrases)+len(q.Short))
	parts = append(parts, q.Words...)
	parts = append(parts, q.Phrases...)
	parts = append(parts, q.Short...)
	return strings.Join(parts, " ")
}

func excludeFunc(q Query) func(string) bool {
	if len(q.Exclude) == 0 {
		return nil
	}
	return keepFunc(Query{Exclude: q.Exclude})
}
