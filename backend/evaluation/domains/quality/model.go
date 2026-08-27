package quality

import (
	"context"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid quality run")
var ErrNotFound = errors.New("quality run not found")

type CaseResult struct {
	CaseID, Category                                                                                                                                                                  string
	ExpectedBlocking, DetectedBlocking, TruePositiveFindings, FalsePositiveFindings, EvidenceChecks, CorrectEvidence, CitationChecks, CorrectCitations, VerdictRuns, MatchingVerdicts uint32
	Exceptions                                                                                                                                                                        []string
}
type Metrics struct{ BlockingRecall, FindingPrecision, EvidenceAccuracy, CitationAccuracy, VerdictConsistency float64 }
type Thresholds = Metrics
type Delta struct {
	Metric                   string
	Baseline, Current, Delta float64
}
type Run struct {
	ID, DatasetVersion, Model, PromptVersion, CriteriaVersion, BaselineRunID string
	Cases                                                                    []CaseResult
	Metrics                                                                  Metrics
	Thresholds                                                               Thresholds
	ReleaseGatePassed                                                        bool
	Failures                                                                 []string
	Deltas                                                                   []Delta
	CreatedAt                                                                time.Time
}
type Store interface {
	Save(context.Context, Run) (Run, error)
	Get(context.Context, string) (Run, error)
}
