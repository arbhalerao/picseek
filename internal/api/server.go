package api

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
	"github.com/arbhalerao/picseek/internal/daemon"
	"github.com/arbhalerao/picseek/internal/search"
)

const statusInterval = time.Second

type Server struct {
	seekdv1.UnimplementedSeekdServiceServer
	daemon *daemon.Daemon
}

func NewServer(d *daemon.Daemon) *Server {
	return &Server{daemon: d}
}

func (s *Server) AddFolders(ctx context.Context, req *seekdv1.AddFoldersRequest) (*seekdv1.AddFoldersResponse, error) {
	added, err := s.daemon.AddFolders(ctx, req.GetPaths())
	if err != nil {
		return nil, toStatus(err)
	}
	return &seekdv1.AddFoldersResponse{Added: added}, nil
}

func (s *Server) ForgetFolder(ctx context.Context, req *seekdv1.ForgetFolderRequest) (*seekdv1.ForgetFolderResponse, error) {
	removed, err := s.daemon.ForgetFolder(ctx, req.GetPath())
	if err != nil {
		return nil, toStatus(err)
	}
	return &seekdv1.ForgetFolderResponse{RemovedPaths: removed}, nil
}

func (s *Server) RetryFailed(ctx context.Context, _ *seekdv1.RetryFailedRequest) (*seekdv1.RetryFailedResponse, error) {
	n, err := s.daemon.RetryFailed(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &seekdv1.RetryFailedResponse{Requeued: n}, nil
}

func (s *Server) Search(ctx context.Context, req *seekdv1.SearchRequest) (*seekdv1.SearchResponse, error) {
	layers := make([]search.MatchKind, 0, len(req.GetLayers()))
	for _, l := range req.GetLayers() {
		layers = append(layers, fromMatchKind(l))
	}
	result, err := s.daemon.Search(ctx, req.GetQuery(), int(req.GetLimit()), layers)
	if err != nil {
		return nil, toStatus(err)
	}
	hits := make([]*seekdv1.Hit, len(result.Hits))
	for i, h := range result.Hits {
		matches := make([]seekdv1.MatchKind, len(h.Matches))
		for j, m := range h.Matches {
			matches[j] = matchKind(m)
		}
		hits[i] = &seekdv1.Hit{Path: h.Path, Rank: int32(h.Rank), Best: matchKind(h.Best), Matches: matches, Score: h.Score}
	}
	return &seekdv1.SearchResponse{Hits: hits, Took: durationpb.New(result.Took)}, nil
}

func (s *Server) Status(ctx context.Context, _ *seekdv1.StatusRequest) (*seekdv1.StatusResponse, error) {
	st, err := s.status(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &seekdv1.StatusResponse{Status: st}, nil
}

func (s *Server) WatchStatus(_ *seekdv1.WatchStatusRequest, stream seekdv1.SeekdService_WatchStatusServer) error {
	ticker := time.NewTicker(statusInterval)
	defer ticker.Stop()
	for {
		st, err := s.status(stream.Context())
		if err != nil {
			return toStatus(err)
		}
		if err := stream.Send(&seekdv1.WatchStatusResponse{Status: st}); err != nil {
			return err
		}
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Server) status(ctx context.Context) (*seekdv1.Status, error) {
	st, err := s.daemon.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &seekdv1.Status{
		Folders:         st.Folders,
		Indexed:         st.Counts.Indexed,
		QueuedLive:      st.Counts.QueuedLive,
		QueuedBackfill:  st.Counts.QueuedBackfill,
		Failed:          st.Counts.Failed,
		ImagesPerSecond: st.Rate,
		Eta:             durationpb.New(st.ETA),
		Skipped:         st.Skipped,
		QueuedEnrich:    st.Counts.QueuedEnrich,
		Services: []*seekdv1.ServiceHealth{
			{Name: "ocr", Healthy: int32(st.OCRHealthy), Total: int32(st.OCRTotal)},
			{Name: "embed", Healthy: boolInt(st.EmbedHealthy), Total: 1},
		},
	}, nil
}

func matchKind(k search.MatchKind) seekdv1.MatchKind {
	switch k {
	case search.MatchExact:
		return seekdv1.MatchKind_MATCH_KIND_EXACT
	case search.MatchNear:
		return seekdv1.MatchKind_MATCH_KIND_NEAR
	case search.MatchMeaning:
		return seekdv1.MatchKind_MATCH_KIND_MEANING
	case search.MatchVisual:
		return seekdv1.MatchKind_MATCH_KIND_VISUAL
	default:
		return seekdv1.MatchKind_MATCH_KIND_UNSPECIFIED
	}
}

func fromMatchKind(k seekdv1.MatchKind) search.MatchKind {
	switch k {
	case seekdv1.MatchKind_MATCH_KIND_NEAR:
		return search.MatchNear
	case seekdv1.MatchKind_MATCH_KIND_MEANING:
		return search.MatchMeaning
	case seekdv1.MatchKind_MATCH_KIND_VISUAL:
		return search.MatchVisual
	default:
		return search.MatchExact
	}
}

func boolInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func toStatus(err error) error {
	var qe *search.QueryError
	switch {
	case errors.As(err, &qe), errors.Is(err, daemon.ErrInvalidFolder):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, daemon.ErrConfigFolder):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, daemon.ErrUnknownFolder):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
