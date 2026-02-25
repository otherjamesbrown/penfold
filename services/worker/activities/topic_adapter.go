package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/pkg/topics"
)

// TopicLookupAdapter adapts topics.Repository to the TopicLookupInterface
// used by the context builder activities.
type TopicLookupAdapter struct {
	repo *topics.Repository
}

// NewTopicLookupAdapter creates a new adapter wrapping a topics repository.
func NewTopicLookupAdapter(repo *topics.Repository) *TopicLookupAdapter {
	return &TopicLookupAdapter{repo: repo}
}

func (a *TopicLookupAdapter) GetByName(ctx context.Context, tenantID, name string) (TopicResult, error) {
	t, err := a.repo.GetByName(ctx, tenantID, name)
	if err != nil {
		return TopicResult{}, err
	}
	return toTopicResult(t), nil
}

func (a *TopicLookupAdapter) ResolveByKeyword(ctx context.Context, tenantID, keyword string) (*int64, error) {
	return a.repo.ResolveByKeyword(ctx, tenantID, keyword)
}

func (a *TopicLookupAdapter) GetByID(ctx context.Context, id int64) (TopicResult, error) {
	t, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return TopicResult{}, err
	}
	return toTopicResult(t), nil
}

func (a *TopicLookupAdapter) ListForContext(ctx context.Context, tenantID string, names []string) ([]TopicResult, error) {
	ts, err := a.repo.ListForContext(ctx, tenantID, names)
	if err != nil {
		return nil, err
	}
	results := make([]TopicResult, len(ts))
	for i, t := range ts {
		results[i] = toTopicResult(t)
	}
	return results, nil
}

func toTopicResult(t *topics.Topic) TopicResult {
	desc := ""
	if t.Description != nil {
		desc = *t.Description
	}
	return TopicResult{
		ID:          t.ID,
		Name:        t.Name,
		Description: desc,
	}
}
