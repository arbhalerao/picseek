package search

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arbhalerao/picseek/internal/store"
)

type MatchKind int

const (
	MatchFilter MatchKind = iota
	MatchExact
	MatchNear
	MatchMeaning
	MatchVisual
)

func (k MatchKind) String() string {
	return [...]string{"filter", "exact", "near", "meaning", "visual"}[k]
}

type Candidate struct {
	ImageID int64
	Kind    MatchKind
	Score   float64
	TakenAt int64
}

type Retriever interface {
	Retrieve(ctx context.Context, q Query, limit int) ([]Candidate, error)
}

type Hit struct {
	Path    string
	Rank    int
	Best    MatchKind
	Matches []MatchKind
	Score   float64
}

type Result struct {
	Query Query
	Hits  []Hit
	Took  time.Duration
}

type Engine struct {
	store      *store.Store
	retrievers []Retriever
}

func NewEngine(st *store.Store, retrievers ...Retriever) *Engine {
	return &Engine{store: st, retrievers: retrievers}
}

func (e *Engine) Search(ctx context.Context, raw string, limit int) (Result, error) {
	started := time.Now()
	q, err := Parse(raw, started)
	if err != nil {
		return Result{}, err
	}

	sets := make([][]Candidate, len(e.retrievers))
	g, gctx := errgroup.WithContext(ctx)
	for i, r := range e.retrievers {
		g.Go(func() error {
			found, err := r.Retrieve(gctx, q, limit)
			sets[i] = found
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return Result{}, err
	}

	ranked := rank(sets)
	ids := make([]int64, len(ranked))
	for i, c := range ranked {
		ids[i] = c.best.ImageID
	}
	paths, err := e.store.PathsFor(ctx, ids, q.Folder)
	if err != nil {
		return Result{}, err
	}

	var hits []Hit
	for _, c := range ranked {
		for _, p := range paths[c.best.ImageID] {
			if len(hits) == limit {
				break
			}
			hits = append(hits, Hit{Path: p, Rank: len(hits) + 1, Best: c.best.Kind, Matches: c.kinds, Score: c.best.Score})
		}
	}
	return Result{Query: q, Hits: hits, Took: time.Since(started)}, nil
}

type ranked struct {
	best  Candidate
	kinds []MatchKind
	order int
}

func rank(sets [][]Candidate) []ranked {
	byImage := make(map[int64]*ranked)
	var out []*ranked
	for _, set := range sets {
		for i, c := range set {
			r, ok := byImage[c.ImageID]
			if !ok {
				r = &ranked{best: c, order: i}
				byImage[c.ImageID] = r
				out = append(out, r)
			} else if tier(c.Kind) < tier(r.best.Kind) {
				r.best, r.order = c, i
			}
			if !slices.Contains(r.kinds, c.Kind) {
				r.kinds = append(r.kinds, c.Kind)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b *ranked) int {
		if ta, tb := tier(a.best.Kind), tier(b.best.Kind); ta != tb {
			return ta - tb
		}
		return a.order - b.order
	})
	result := make([]ranked, len(out))
	for i, r := range out {
		result[i] = *r
	}
	return result
}

func tier(k MatchKind) int {
	switch k {
	case MatchExact, MatchFilter:
		return 0
	case MatchNear:
		return 1
	default:
		return 2
	}
}

type ExactRetriever struct {
	store *store.Store
}

func NewExactRetriever(st *store.Store) *ExactRetriever {
	return &ExactRetriever{store: st}
}

func (r *ExactRetriever) Retrieve(ctx context.Context, q Query, limit int) ([]Candidate, error) {
	terms := make([]string, 0, len(q.Words)+len(q.Phrases))
	for _, t := range append(slices.Clone(q.Words), q.Phrases...) {
		terms = append(terms, `"`+strings.ReplaceAll(t, `"`, `""`)+`"`)
	}
	kind := MatchExact
	if !q.HasText() {
		kind = MatchFilter
	}
	hits, err := r.store.SearchText(ctx, store.TextQuery{
		Match:  strings.Join(terms, " AND "),
		Folder: q.Folder,
		After:  q.After,
		Before: q.Before,
		Type:   q.Type,
		Keep:   keepFunc(q),
		Limit:  limit,
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

func keepFunc(q Query) func(string) bool {
	if len(q.Short) == 0 && len(q.Exclude) == 0 {
		return nil
	}
	shorts := make([]*regexp.Regexp, len(q.Short))
	for i, w := range q.Short {
		shorts[i] = regexp.MustCompile(`(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(w) + `($|[^\pL\pN])`)
	}
	excludes := make([]string, len(q.Exclude))
	for i, w := range q.Exclude {
		excludes[i] = strings.ToLower(w)
	}
	return func(text string) bool {
		for _, re := range shorts {
			if !re.MatchString(text) {
				return false
			}
		}
		if len(excludes) == 0 {
			return true
		}
		lower := strings.ToLower(text)
		for _, w := range excludes {
			if strings.Contains(lower, w) {
				return false
			}
		}
		return true
	}
}
