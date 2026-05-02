package feedback

import (
	"context"
	"errors"

	"github.com/mitpoai/pookiepaws/internal/memory"
)

type Service struct {
	store *memory.Store
}

func NewService(store *memory.Store) Service {
	return Service{store: store}
}

func (s Service) Add(ctx context.Context, projectID string, score int, corrections, lessons string) (memory.Feedback, error) {
	if s.store == nil {
		return memory.Feedback{}, errors.New("feedback service requires a memory store")
	}
	return s.store.AddFeedback(ctx, projectID, score, corrections, lessons)
}
