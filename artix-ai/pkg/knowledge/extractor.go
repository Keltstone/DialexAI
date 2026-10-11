package knowledge

import (
	"fmt"
	"strings"
	"time"

	"artix/pkg/spec"
)

// ConvergenceInfo contains convergence summary metrics for learning extraction.
type ConvergenceInfo struct {
	Success   bool
	RoundsRun int
}

// LearningExtractor synthesizes institutional knowledge from converged coding sessions.
type LearningExtractor struct {
	store *Store
}

// NewLearningExtractor creates an extractor.
func NewLearningExtractor(store *Store) *LearningExtractor {
	return &LearningExtractor{store: store}
}

// ExtractFromConvergence inspects a completed coder loop result.
// If the loop required 2 or more rounds to resolve test/reviewer failures, it creates a Knowledge Item.
func (le *LearningExtractor) ExtractFromConvergence(s *spec.StorySpec, res ConvergenceInfo) (*KnowledgeItem, error) {
	if !res.Success {
		return nil, fmt.Errorf("cannot extract learning from uncompleted run")
	}

	if res.RoundsRun < 2 {
		return nil, nil // First-round pass does not imply tricky edge-case learning
	}

	title := fmt.Sprintf("Convergence insight for %s: %s", s.ID, s.Title)
	breakthrough := fmt.Sprintf("Resolved initial reviewer/test rejections in round %d. Code now conforms to acceptance criteria and passes: %s", res.RoundsRun, strings.Join(s.TestCommands, ", "))

	ki := &KnowledgeItem{
		ID:           fmt.Sprintf("ki-conv-%d", time.Now().Unix()),
		Title:        title,
		Category:     CategoryDebugging,
		Context:      s.UserStory,
		Breakthrough: breakthrough,
		CreatedAt:    time.Now(),
	}

	if err := le.store.Save(ki); err != nil {
		return nil, err
	}

	return ki, nil
}
