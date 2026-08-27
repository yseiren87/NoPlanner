package assessment

import "time"

type Location struct {
	PageNumber      uint32   `json:"page_number,omitempty"`
	SlideNumber     uint32   `json:"slide_number,omitempty"`
	SheetName       string   `json:"sheet_name,omitempty"`
	SectionPath     []string `json:"section_path,omitempty"`
	ParagraphNumber uint32   `json:"paragraph_number,omitempty"`
}

type Assumption struct {
	ID, Statement, Criticality, CriticalityReason, FailureImpact string
	Confidence                                                   float64
	SourceBlockOrdinal                                           uint32
	Location                                                     Location
}

type Risk struct {
	ID, Statement, FailureImpact                  string
	Likelihood, Impact, Reversibility, Confidence float64
	AssumptionIDs                                 []string
	SourceBlockOrdinal                            uint32
	Location                                      Location
}

type Analysis struct {
	ID, DocumentID, Model string
	DocumentVersion       uint32
	Assumptions           []Assumption
	Risks                 []Risk
	CreatedAt             time.Time
}

type ValidationPlanItem struct {
	Area      string   `json:"area"`
	Selected  bool     `json:"selected"`
	Depth     string   `json:"depth"`
	Reason    string   `json:"reason"`
	DriverIDs []string `json:"driver_ids"`
}

type ValidationPlan struct {
	ID, DocumentID, Purpose, Country, Domain, Model string
	DocumentVersion                                 uint32
	Items                                           []ValidationPlanItem
	CreatedAt                                       time.Time
}

type ValidationResult struct {
	Area, Status, Reason    string
	CountedAsDefect, Scored bool
}

type ValidationResultSummary struct {
	ValidationPlanID                        string
	Results                                 []ValidationResult
	DefectCount, ScoredCount, ExcludedCount uint32
	SatisfactionRate                        float64
	CreatedAt                               time.Time
}

type Finding struct {
	ID, Type, Statement, Finding, ReasoningSummary, Impact, Severity, RequiredAction string
	Confidence                                                                       float64
	SourceBlockOrdinals                                                              []uint32
	SourceLocations                                                                  []Location
	DocumentAbsence                                                                  bool
}

type PurposeAlignmentEvaluation struct {
	ID, DocumentID, Status, Model string
	DocumentVersion               uint32
	Findings                      []Finding
	CreatedAt                     time.Time
}

type Contradiction struct {
	Finding
	Category, FirstStatement, SecondStatement string
}

type ContradictionEvaluation struct {
	ID, DocumentID, Status, Model string
	DocumentVersion               uint32
	Contradictions                []Contradiction
	CreatedAt                     time.Time
}

type RequirementGap struct {
	Finding
	Category, MissingDecision string
}

type RequirementCompletenessEvaluation struct {
	ID, DocumentID, Status, Model string
	DocumentVersion               uint32
	Gaps                          []RequirementGap
	CreatedAt                     time.Time
}

type WorkQualityResult struct {
	Dimension, Status, Finding, ReasoningSummary string
	Confidence                                   float64
	SourceBlockOrdinals                          []uint32
	SourceLocations                              []Location
}

type DocumentWorkQualityEvaluation struct {
	ID, DocumentID, Model string
	DocumentVersion       uint32
	Results               []WorkQualityResult
	CreatedAt             time.Time
}
