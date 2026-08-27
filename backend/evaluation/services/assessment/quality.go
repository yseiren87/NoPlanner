package assessment

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	qualitydomain "noplanner/backend/evaluation/domains/quality"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	"time"
)

func (s *Service) RecordQualityRun(ctx context.Context, req *evaluationv1.RecordQualityRunRequest) (*evaluationv1.QualityRun, error) {
	if s.quality == nil {
		return nil, status.Error(codes.FailedPrecondition, "품질 저장소가 설정되지 않았습니다.")
	}
	run := qualitydomain.Run{ID: fmt.Sprintf("quality-%d", time.Now().UnixNano()), DatasetVersion: req.GetDatasetVersion(), Model: req.GetModel(), PromptVersion: req.GetPromptVersion(), CriteriaVersion: req.GetCriteriaVersion(), BaselineRunID: req.GetBaselineRunId(), Thresholds: thresholdsDomain(req.GetThresholds())}
	for _, item := range req.GetCases() {
		run.Cases = append(run.Cases, qualitydomain.CaseResult{CaseID: item.GetCaseId(), Category: item.GetCategory(), ExpectedBlocking: item.GetExpectedBlocking(), DetectedBlocking: item.GetDetectedBlocking(), TruePositiveFindings: item.GetTruePositiveFindings(), FalsePositiveFindings: item.GetFalsePositiveFindings(), EvidenceChecks: item.GetEvidenceChecks(), CorrectEvidence: item.GetCorrectEvidence(), CitationChecks: item.GetCitationChecks(), CorrectCitations: item.GetCorrectCitations(), VerdictRuns: item.GetVerdictRuns(), MatchingVerdicts: item.GetMatchingVerdicts(), Exceptions: item.GetExceptions()})
	}
	var baseline *qualitydomain.Run
	if req.GetBaselineRunId() != "" {
		value, err := s.quality.Get(ctx, req.GetBaselineRunId())
		if err != nil {
			return nil, status.Error(codes.NotFound, "기준 품질 실행을 찾을 수 없습니다.")
		}
		baseline = &value
	}
	evaluated, err := qualitydomain.Evaluate(run, baseline)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "품질 실행 입력이 유효하지 않습니다.")
	}
	saved, err := s.quality.Save(ctx, evaluated)
	if err != nil {
		return nil, status.Error(codes.Internal, "품질 실행을 저장하지 못했습니다.")
	}
	return qualityProto(saved), nil
}
func (s *Service) GetQualityRun(ctx context.Context, req *evaluationv1.GetQualityRunRequest) (*evaluationv1.QualityRun, error) {
	if s.quality == nil {
		return nil, status.Error(codes.FailedPrecondition, "품질 저장소가 설정되지 않았습니다.")
	}
	run, err := s.quality.Get(ctx, req.GetQualityRunId())
	if errors.Is(err, qualitydomain.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "품질 실행을 찾을 수 없습니다.")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "품질 실행을 불러오지 못했습니다.")
	}
	return qualityProto(run), nil
}
func thresholdsDomain(value *evaluationv1.QualityThresholds) qualitydomain.Thresholds {
	if value == nil {
		return qualitydomain.DefaultThresholds()
	}
	return qualitydomain.Thresholds{BlockingRecall: value.GetBlockingRecall(), FindingPrecision: value.GetFindingPrecision(), EvidenceAccuracy: value.GetEvidenceAccuracy(), CitationAccuracy: value.GetCitationAccuracy(), VerdictConsistency: value.GetVerdictConsistency()}
}
func qualityProto(run qualitydomain.Run) *evaluationv1.QualityRun {
	out := &evaluationv1.QualityRun{Id: run.ID, DatasetVersion: run.DatasetVersion, Model: run.Model, PromptVersion: run.PromptVersion, CriteriaVersion: run.CriteriaVersion, BaselineRunId: run.BaselineRunID, Metrics: &evaluationv1.QualityMetrics{BlockingRecall: run.Metrics.BlockingRecall, FindingPrecision: run.Metrics.FindingPrecision, EvidenceAccuracy: run.Metrics.EvidenceAccuracy, CitationAccuracy: run.Metrics.CitationAccuracy, VerdictConsistency: run.Metrics.VerdictConsistency}, Thresholds: &evaluationv1.QualityThresholds{BlockingRecall: run.Thresholds.BlockingRecall, FindingPrecision: run.Thresholds.FindingPrecision, EvidenceAccuracy: run.Thresholds.EvidenceAccuracy, CitationAccuracy: run.Thresholds.CitationAccuracy, VerdictConsistency: run.Thresholds.VerdictConsistency}, ReleaseGatePassed: run.ReleaseGatePassed, Failures: run.Failures, CreatedAt: timestamppb.New(run.CreatedAt)}
	for _, item := range run.Cases {
		out.Cases = append(out.Cases, &evaluationv1.GoldenCaseResult{CaseId: item.CaseID, Category: item.Category, ExpectedBlocking: item.ExpectedBlocking, DetectedBlocking: item.DetectedBlocking, TruePositiveFindings: item.TruePositiveFindings, FalsePositiveFindings: item.FalsePositiveFindings, EvidenceChecks: item.EvidenceChecks, CorrectEvidence: item.CorrectEvidence, CitationChecks: item.CitationChecks, CorrectCitations: item.CorrectCitations, VerdictRuns: item.VerdictRuns, MatchingVerdicts: item.MatchingVerdicts, Exceptions: item.Exceptions})
	}
	for _, item := range run.Deltas {
		out.Deltas = append(out.Deltas, &evaluationv1.QualityMetricDelta{Metric: item.Metric, Baseline: item.Baseline, Current: item.Current, Delta: item.Delta})
	}
	return out
}
