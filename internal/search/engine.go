package search

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	"github.com/arbhalerao/picseek/internal/store"
	"github.com/arbhalerao/picseek/internal/textnorm"
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

type Layer struct {
	Kind      MatchKind
	Retriever Retriever
}

type Engine struct {
	store   *store.Store
	typeMin float64
	layers  []Layer
}

func NewEngine(st *store.Store, typeMin float64, layers ...Layer) *Engine {
	return &Engine{store: st, typeMin: typeMin, layers: layers}
}

func (e *Engine) Search(ctx context.Context, raw string, limit int, only []MatchKind) (Result, error) {
	started := time.Now()
	q, err := Parse(raw, started)
	if err != nil {
		return Result{}, err
	}
	q.TypeMin = e.typeMin

	var active []Retriever
	for _, l := range e.layers {
		if len(only) == 0 || slices.Contains(only, l.Kind) {
			active = append(active, l.Retriever)
		}
	}
	sets := make([][]Candidate, len(active))
	g, gctx := errgroup.WithContext(ctx)
	for i, r := range active {
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
	order float64
}

const rrfK = 60

func rank(sets [][]Candidate) []ranked {
	byImage := make(map[int64]*ranked)
	var out []*ranked
	for _, set := range sets {
		for i, c := range set {
			r, ok := byImage[c.ImageID]
			if !ok {
				r = &ranked{best: c}
				byImage[c.ImageID] = r
				out = append(out, r)
			}
			switch {
			case !ok:
				r.order = position(c.Kind, i)
			case tier(c.Kind) < tier(r.best.Kind):
				r.best, r.order = c, position(c.Kind, i)
			case tier(c.Kind) == tier(r.best.Kind) && tier(c.Kind) == semanticTier:
				r.order += position(c.Kind, i)
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
		switch {
		case a.order < b.order:
			return -1
		case a.order > b.order:
			return 1
		default:
			return 0
		}
	})
	result := make([]ranked, len(out))
	for i, r := range out {
		result[i] = *r
	}
	return result
}

func position(k MatchKind, index int) float64 {
	if tier(k) == semanticTier {
		return -1 / float64(rrfK+index+1)
	}
	return float64(index)
}

const semanticTier = 2

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
	kind := MatchExact
	if !q.HasText() {
		kind = MatchFilter
	}
	return retrieve(ctx, r.store, q, exactExpr(q), kind, limit)
}

type NearRetriever struct {
	store *store.Store
}

func NewNearRetriever(st *store.Store) *NearRetriever {
	return &NearRetriever{store: st}
}

func (r *NearRetriever) Retrieve(ctx context.Context, q Query, limit int) ([]Candidate, error) {
	exact := exactExpr(q)
	if exact == "" {
		return nil, nil
	}
	var clauses []string
	for _, t := range textTerms(q) {
		folded := textnorm.Fold(t)
		if utf8.RuneCountInString(folded) < minTrigram {
			continue
		}
		clauses = append(clauses, "{folded compact} : "+quote(folded))
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	expr := "(" + strings.Join(clauses, " AND ") + ") NOT (" + exact + ")"
	return retrieve(ctx, r.store, q, expr, MatchNear, limit)
}

func retrieve(ctx context.Context, st *store.Store, q Query, match string, kind MatchKind, limit int) ([]Candidate, error) {
	hits, err := st.SearchText(ctx, store.TextQuery{
		Match:   match,
		Folder:  q.Folder,
		After:   q.After,
		Before:  q.Before,
		Type:    q.Type,
		TypeMin: q.TypeMin,
		Keep:    keepFunc(q),
		Limit:   limit,
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

func exactExpr(q Query) string {
	terms := textTerms(q)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = quote(t)
	}
	return "text : (" + strings.Join(quoted, " AND ") + ")"
}

func textTerms(q Query) []string {
	return append(slices.Clone(q.Words), q.Phrases...)
}

func quote(term string) string {
	return `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
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
