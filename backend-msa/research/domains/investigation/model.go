package investigation

import (
	"context"
	"time"
)

type Assumption struct {
	ID, Statement, FailureImpact string
	BlockingLikelihood           float64
	DependencyTypes              []string
}

type Limit struct {
	MaxQueries, MaxSources, MaxSeconds, SaturationQueries uint32
	MaxCost                                               float64
}

type Task struct {
	ID, AssumptionID, Question, Depth, VerificationType string
	RequiredSourceTypes, Queries                        []string
	Priority                                            uint32
}

type Plan struct {
	ID, EvaluationID, TargetCountry, TargetDomain string
	Tasks                                         []Task
	Limit                                         Limit
	CreatedAt                                     time.Time
}

type SearchTrace struct {
	Query                   string
	ResultURLs, VisitedURLs []string
}

type Evidence struct {
	ID, TaskID, Title, Publisher, SourceType, URL, OriginalURL string
	PublishedAt, VerifiedAt, TargetCountry, TargetDomain       string
	ApplicableScope, SourceLocation, Excerpt, Relation         string
	ReliabilityGrade, AccessStatus, PricingStatus              string
	CommercialUseStatus, LicenseOrTermsURL, ContentHash        string
	UsageRestrictions, Limitations                             []string
	OfficialSource, SampleCallVerified                         bool
}

type Result struct {
	TaskID, AssumptionID, VerificationType, Status, Conclusion string
	EvidenceIDs, ConflictingEvidenceIDs, DimensionResults      []string
	RemainingUncertainties                                     []string
}

type Report struct {
	ID, PlanID, EvaluationID, Status         string
	SearchTraces                             []SearchTrace
	Evidence                                 []Evidence
	Results                                  []Result
	QueriesUsed, SourcesUsed, ElapsedSeconds uint32
	EstimatedCost                            float64
	LimitReasons, RemainingUncertainties     []string
	CreatedAt                                time.Time
}

type SearchHit struct{ Title, URL, Snippet, Publisher string }
type Page struct{ URL, Title, Text, SourceLocation, ContentHash, ContentType string }

type Searcher interface {
	Search(query string) ([]SearchHit, error)
	Fetch(url string) (Page, error)
}
type Store interface {
	SavePlan(context.Context, Plan) (Plan, error)
	SaveReport(context.Context, Report) (Report, error)
	GetReport(context.Context, string) (Report, error)
}
