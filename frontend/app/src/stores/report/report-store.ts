import { createImprovementPlan, getReport, getVerdictHistory, improveExistingPlan, reviseReport, type EvaluationReport, type ImprovedPlan, type ImprovementPlan, type RevisionResult, type VerdictHistory } from "@/api/reports";

export type ReportState = {
  report: EvaluationReport | null;
  revision: RevisionResult | null;
  improvementPlan: ImprovementPlan | null;
  improvedPlan: ImprovedPlan | null;
  verdictHistory: VerdictHistory | null;
  loading: boolean;
  revising: boolean;
  error: string | null;
};

export class ReportStore {
  state: ReportState = { report: null, revision: null, improvementPlan: null, improvedPlan: null, verdictHistory: null, loading: false, revising: false, error: null };
  listeners = new Set<() => void>();
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };
  snapshot = () => this.state;

  async load(id: string) {
    this.state = { ...this.state, loading: true, error: null };
    this.emit();
    try {
      const [report, verdictHistory] = await Promise.all([getReport(id), getVerdictHistory(id)]);
      this.state = { ...this.state, report, revision: null, improvementPlan: null, improvedPlan: null, verdictHistory, loading: false, error: null };
    } catch (error) {
      this.state = { ...this.state, loading: false, error: error instanceof Error ? error.message : "오류가 발생했습니다." };
    }
    this.emit();
  }

  async revise(file: File) {
    const reportID = this.state.report?.id;
    if (!reportID) return;
    this.state = { ...this.state, revising: true, error: null };
    this.emit();
    try {
      const revision = await reviseReport(reportID, file);
      this.state = { ...this.state, report: revision.report, revision, revising: false, error: null };
    } catch (error) {
      this.state = { ...this.state, revising: false, error: error instanceof Error ? error.message : "수정본 재평가 중 오류가 발생했습니다." };
    }
    this.emit();
  }

  async planImprovements() {
    const reportID = this.state.report?.id;
    if (!reportID) return;
    this.state = { ...this.state, revising: true, error: null };
    this.emit();
    try {
      this.state = { ...this.state, improvementPlan: await createImprovementPlan(reportID), revising: false };
    } catch (error) {
      this.state = { ...this.state, revising: false, error: error instanceof Error ? error.message : "보완 계획 생성 중 오류가 발생했습니다." };
    }
    this.emit();
  }

  async improve(claim: { statement: string; evidenceIds: string[]; findingIds: string[] }) {
    const report = this.state.report;
    if (!report) return;
    const originalSections = Object.entries(report.summary).map(([key, content]) => ({ key, title: key, content }));
    this.state = { ...this.state, revising: true, error: null };
    this.emit();
    try {
      const improvedPlan = await improveExistingPlan(report.id, { originalIntent: report.summary.purpose, originalSections, claims: [{ ...claim, verified: true }] });
      this.state = { ...this.state, improvedPlan, revising: false };
    } catch (error) {
      this.state = { ...this.state, revising: false, error: error instanceof Error ? error.message : "개선본 생성 중 오류가 발생했습니다." };
    }
    this.emit();
  }

  private emit() {
    this.listeners.forEach((listener) => listener());
  }
}

export const reportStore = new ReportStore();
