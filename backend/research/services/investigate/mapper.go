package investigate

import (
	"errors"

	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	"noplanner/backend/research/domains/investigation"
)

func fromProtoLimit(value *researchv1.ResearchLimit) investigation.Limit {
	if value == nil {
		return investigation.Limit{}
	}
	return investigation.Limit{MaxQueries: value.GetMaxQueries(), MaxSources: value.GetMaxSources(), MaxSeconds: value.GetMaxSeconds(), MaxCost: value.GetMaxCost(), SaturationQueries: value.GetSaturationQueries()}
}

func toProtoLimit(value investigation.Limit) *researchv1.ResearchLimit {
	return &researchv1.ResearchLimit{MaxQueries: value.MaxQueries, MaxSources: value.MaxSources, MaxSeconds: value.MaxSeconds, MaxCost: value.MaxCost, SaturationQueries: value.SaturationQueries}
}

func fromProtoPlan(value *researchv1.ResearchPlan) (investigation.Plan, error) {
	if value == nil || value.GetId() == "" || value.GetEvaluationId() == "" || len(value.GetTasks()) == 0 {
		return investigation.Plan{}, errors.New("invalid plan")
	}
	plan := investigation.Plan{ID: value.GetId(), EvaluationID: value.GetEvaluationId(), TargetCountry: value.GetTargetCountry(), TargetDomain: value.GetTargetDomain(), Limit: fromProtoLimit(value.GetLimit())}
	for _, task := range value.GetTasks() {
		if task.GetId() == "" || task.GetAssumptionId() == "" {
			return investigation.Plan{}, errors.New("invalid task")
		}
		plan.Tasks = append(plan.Tasks, investigation.Task{ID: task.GetId(), AssumptionID: task.GetAssumptionId(), Question: task.GetQuestion(), RequiredSourceTypes: task.GetRequiredSourceTypes(), Depth: task.GetDepth(), Priority: task.GetPriority(), Queries: task.GetQueries(), VerificationType: task.GetVerificationType()})
	}
	return plan, nil
}

func toProtoPlan(value investigation.Plan) *researchv1.ResearchPlan {
	result := &researchv1.ResearchPlan{Id: value.ID, EvaluationId: value.EvaluationID, TargetCountry: value.TargetCountry, TargetDomain: value.TargetDomain, Limit: toProtoLimit(value.Limit)}
	for _, task := range value.Tasks {
		result.Tasks = append(result.Tasks, &researchv1.ResearchTask{Id: task.ID, AssumptionId: task.AssumptionID, Question: task.Question, RequiredSourceTypes: task.RequiredSourceTypes, Depth: task.Depth, Priority: task.Priority, Queries: task.Queries, VerificationType: task.VerificationType})
	}
	return result
}

func toProtoReport(value investigation.Report) *researchv1.ResearchReport {
	result := &researchv1.ResearchReport{Id: value.ID, PlanId: value.PlanID, EvaluationId: value.EvaluationID, Status: value.Status, QueriesUsed: value.QueriesUsed, SourcesUsed: value.SourcesUsed, ElapsedSeconds: value.ElapsedSeconds, EstimatedCost: value.EstimatedCost, LimitReasons: value.LimitReasons, RemainingUncertainties: value.RemainingUncertainties}
	for _, trace := range value.SearchTraces {
		result.SearchTraces = append(result.SearchTraces, &researchv1.SearchTrace{Query: trace.Query, ResultUrls: trace.ResultURLs, VisitedUrls: trace.VisitedURLs})
	}
	for _, item := range value.Evidence {
		result.Evidence = append(result.Evidence, &researchv1.SourceEvidence{Id: item.ID, TaskId: item.TaskID, Title: item.Title, Publisher: item.Publisher, SourceType: item.SourceType, Url: item.URL, OriginalUrl: item.OriginalURL, PublishedAt: item.PublishedAt, VerifiedAt: item.VerifiedAt, TargetCountry: item.TargetCountry, TargetDomain: item.TargetDomain, ApplicableScope: item.ApplicableScope, SourceLocation: item.SourceLocation, Excerpt: item.Excerpt, Relation: item.Relation, ReliabilityGrade: item.ReliabilityGrade, AccessStatus: item.AccessStatus, PricingStatus: item.PricingStatus, CommercialUseStatus: item.CommercialUseStatus, LicenseOrTermsUrl: item.LicenseOrTermsURL, UsageRestrictions: item.UsageRestrictions, Limitations: item.Limitations, ContentHash: item.ContentHash, OfficialSource: item.OfficialSource, SampleCallVerified: item.SampleCallVerified})
	}
	for _, item := range value.Results {
		result.Results = append(result.Results, &researchv1.VerificationResult{TaskId: item.TaskID, AssumptionId: item.AssumptionID, VerificationType: item.VerificationType, Status: item.Status, Conclusion: item.Conclusion, EvidenceIds: item.EvidenceIDs, ConflictingEvidenceIds: item.ConflictingEvidenceIDs, DimensionResults: item.DimensionResults, RemainingUncertainties: item.RemainingUncertainties})
	}
	return result
}
