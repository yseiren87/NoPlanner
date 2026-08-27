import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { ExternalLink, FileDown, LoaderCircle } from "lucide-react";
import { Link, useParams } from "react-router-dom";
import { exportReport, type Finding, type Severity } from "@/api/reports";
import { ObjectionForm } from "@/components/objection-form";
import { RevisionForm } from "@/components/revision-form";
import { reportStore } from "@/stores/report/report-store";

const severityOrder: Record<Severity, number> = { blocking: 4, high: 3, medium: 2, low: 1 };

export function ReportPage() {
  const { id = "" } = useParams();
  const state = useSyncExternalStore(reportStore.subscribe, reportStore.snapshot);
  const [severity, setSeverity] = useState("all");
  const [area, setArea] = useState("all");
  const [status, setStatus] = useState("all");

  useEffect(() => {
    void reportStore.load(id);
  }, [id]);

  const findings = useMemo(
    () =>
      [...(state.report?.findings ?? [])]
        .filter(
          (finding) =>
            (severity === "all" || finding.severity === severity) &&
            (area === "all" || finding.detail.area === area) &&
            (status === "all" || finding.detail.status === status),
        )
        .sort((left, right) => severityOrder[right.severity] - severityOrder[left.severity]),
    [state.report, severity, area, status],
  );

  if (state.loading && !state.report) {
    return <Centered><LoaderCircle className="animate-spin" /> 리포트를 불러오는 중입니다.</Centered>;
  }
  if (state.error) return <Centered>{state.error}</Centered>;
  if (!state.report) return <Centered>리포트가 없습니다.</Centered>;

  const report = state.report;
  return (
    <main className="report-shell">
      <header className="report-header">
        <div>
          <p className="eyebrow">통합 평가 리포트 · {report.documentName} v{report.documentVersion}</p>
          <h1>{verdictLabel(report.verdict)}</h1>
          <p>{report.conclusion}</p>
        </div>
        <div className="export-actions">
          {(["markdown", "pdf", "docx", "json"] as const).map((format) => (
            <button key={format} onClick={() => void exportReport(report.id, format)}>
              <FileDown size={15} />{format.toUpperCase()}
            </button>
          ))}
        </div>
      </header>

      <section className="metric-grid">
        <Metric label="판정 신뢰도" value={report.confidence} />
        <Metric label="구현 시작" value={report.implementationMayStart ? "가능" : "불가"} />
        <Metric label="차단·높음" value={String(report.findings.filter((finding) => finding.severity === "blocking" || finding.severity === "high").length)} />
        <Metric label="평가 상태" value={report.evaluationStatus} />
      </section>

      <Progress steps={report.progress} limitations={report.limitations} />

      <section>
        <h2>기획 재구성</h2>
        <div className="summary-grid">
          {Object.entries({
            문제: report.summary.problem,
            목적: report.summary.purpose,
            "대상 사용자": report.summary.targetUser,
            환경: report.summary.environment,
            해결책: report.summary.solution,
            "성공 기준": report.summary.successCriteria,
          }).map(([key, value]) => <article key={key}><strong>{key}</strong><p>{value || "정의되지 않음"}</p></article>)}
        </div>
      </section>

      <section>
        <div className="section-title">
          <div><h2>문제와 근거</h2><p>작성자와 평가자에게 동일하게 제공되는 결과입니다.</p></div>
          <div className="filters">
            <select aria-label="심각도" value={severity} onChange={(event) => setSeverity(event.target.value)}>
              <option value="all">모든 심각도</option>
              {["blocking", "high", "medium", "low"].map((value) => <option key={value}>{value}</option>)}
            </select>
            <select aria-label="영역" value={area} onChange={(event) => setArea(event.target.value)}>
              <option value="all">모든 영역</option>
              {[...new Set(report.findings.map((finding) => finding.detail.area))].map((value) => <option key={value}>{value}</option>)}
            </select>
            <select aria-label="상태" value={status} onChange={(event) => setStatus(event.target.value)}>
              <option value="all">모든 상태</option>
              {[...new Set(report.findings.map((finding) => finding.detail.status))].map((value) => <option key={value}>{value}</option>)}
            </select>
          </div>
        </div>
        <div className="findings">{findings.map((finding) => <FindingCard key={finding.detail.id} finding={finding} />)}</div>
      </section>

      <section>
        <h2>영역별 평가</h2>
        <div className="area-table">
          {report.areas.map((item) => <div key={item.area}><strong>{item.area}</strong><span>{item.status}</span><span>{item.findingCount}건</span><span>{item.confidence}</span><p>{item.reason}</p></div>)}
        </div>
      </section>

      <section className="two-column">
        <article>
          <h2>전제 및 의존성</h2>
          {report.assumptions.map((assumption) => <div className="compact-card" key={assumption.id}><strong>{assumption.id} · {assumption.status}</strong><p>{assumption.statement}</p><small>의존성: {assumption.dependencies.join(", ") || "없음"} · 근거: {assumption.evidenceIds.join(", ") || "없음"}</small></div>)}
        </article>
        <article>
          <h2>내부 상충</h2>
          {report.contradictions.map((contradiction) => <div className="compact-card" key={contradiction.id}><strong>{contradiction.id}</strong><blockquote>{contradiction.firstLocation}: {contradiction.firstStatement}</blockquote><blockquote>{contradiction.secondLocation}: {contradiction.secondStatement}</blockquote><p>{contradiction.reason}</p></div>)}
        </article>
      </section>

      <section>
        <h2>조사 결과</h2>
        {report.research.map((result, index) => <article className="research-card" key={index}><strong>{result.question}</strong><p>{result.conclusion}</p>{result.sources.map((evidence) => <a key={evidence.id} href={evidence.url} target="_blank" rel="noreferrer"><ExternalLink size={14} />{evidence.id} · {evidence.title || evidence.url} · {evidence.sourceLocation}</a>)}{result.uncertainties.map((value) => <small key={value}>미확인: {value}</small>)}</article>)}
      </section>

      <RevisionForm busy={state.revising} onSubmit={(file) => reportStore.revise(file)} />
      {state.revision && <RevisionSummary revision={state.revision} />}
      <ImprovementSection busy={state.revising} plan={state.improvementPlan} improved={state.improvedPlan} onPlan={() => reportStore.planImprovements()} onImprove={(claim) => reportStore.improve(claim)} />
      <section><h2>판정 변경 이력</h2>{state.verdictHistory?.changes.length ? state.verdictHistory.changes.map((change) => <div className="compact-card" key={change.reevaluationId}><strong>{verdictLabel(change.previousVerdict)} → {verdictLabel(change.currentVerdict)}</strong><p>{change.reasons.join(" · ") || "판정 유지"}</p><small>{change.criteriaVersion} · {change.model} · {new Date(change.changedAt).toLocaleString()}</small></div>) : <p>판정 변경 이력이 없습니다.</p>}</section>
      <ObjectionForm reportId={report.id} findings={report.findings} />
      <section><h2>구현 폐쇄 루프</h2><p>이 평가의 요구사항을 실제 로컬 Git 저장소 및 테스트와 비교합니다.</p><Link to={`/implementation?projectId=${encodeURIComponent(report.projectId)}&evaluationId=${encodeURIComponent(report.evaluationId)}`}>구현 검증 시작</Link></section>

      <section className="recommendation">
        <h2>필수 보완 및 최종 권고</h2>
        {report.requiredActions.map((value) => <p key={value}>필수 · {value}</p>)}
        {report.recommendations.map((value) => <p key={value}>개선 · {value}</p>)}
        {report.verdictChangeConditions.map((value) => <p key={value}>판정 변경 조건 · {value}</p>)}
      </section>
    </main>
  );
}

function ImprovementSection({ busy, plan, improved, onPlan, onImprove }: { busy:boolean; plan:ReturnType<typeof reportStore.snapshot>["improvementPlan"]; improved:ReturnType<typeof reportStore.snapshot>["improvedPlan"]; onPlan:()=>void; onImprove:(claim:{statement:string;evidenceIds:string[];findingIds:string[]})=>void }) {
  const [statement, setStatement] = useState("");
  const [evidenceIDs, setEvidenceIDs] = useState("");
  const [findingIDs, setFindingIDs] = useState("");
  const split = (value:string) => value.split(",").map((item) => item.trim()).filter(Boolean);
  return <section><div className="section-title"><div><h2>객관적 보완 계획</h2><p>Finding을 우선순위와 검증 방법이 있는 작업으로 변환합니다.</p></div><button disabled={busy} onClick={onPlan}>{busy ? "처리 중" : "보완 계획 생성"}</button></div>
    {plan?.tasks.map((task) => <div className="compact-card" key={task.id}><strong>{task.order}. {task.action}{task.blocking ? " · 차단" : ""}</strong><p>검증: {task.verificationMethod}</p><small>Finding: {task.findingIds.join(", ")} · 추가 조사: {task.requiredResearch.join(", ") || "없음"}</small></div>)}
    {plan && <div className="compact-card"><strong>검증된 신규 근거로 개선본 작성</strong><input aria-label="검증된 주장" placeholder="원출처로 확인한 주장" value={statement} onChange={(event) => setStatement(event.target.value)} /><input aria-label="근거 ID" placeholder="근거 ID, 쉼표 구분" value={evidenceIDs} onChange={(event) => setEvidenceIDs(event.target.value)} /><input aria-label="Finding ID" placeholder="해결할 Finding ID, 쉼표 구분" value={findingIDs} onChange={(event) => setFindingIDs(event.target.value)} /><button disabled={busy || !statement.trim() || split(evidenceIDs).length === 0 || split(findingIDs).length === 0} onClick={() => onImprove({statement:statement.trim(),evidenceIds:split(evidenceIDs),findingIds:split(findingIDs)})}>개선본 생성</button></div>}
    {improved && <div className="compact-card"><strong>개선 결과 · 미해결 {improved.unresolvedFindingIds.length}건</strong>{improved.changes.map((change) => <p key={change.sectionKey}>{change.sectionKey}: {change.reason}</p>)}</div>}
  </section>;
}

function RevisionSummary({ revision }: { revision: NonNullable<ReturnType<typeof reportStore.snapshot>["revision"]> }) {
  return <section>
    <h2>버전 비교 및 재평가 결과</h2>
    <p>영향 영역: {revision.comparison.affectedValidationAreas.join(", ") || "없음"}</p>
    <p>판정: {verdictLabel(revision.reevaluation.previousVerdict)} → {verdictLabel(revision.reevaluation.currentVerdict)}</p>
    <p>변경 항목 {revision.comparison.changes.length}건 · 재평가 항목 {revision.reevaluation.findings.length}건</p>
    {revision.reevaluation.verdictChangeReasons.map((reason) => <p key={reason}>판정 변경 근거 · {reason}</p>)}
    <Link to={`/reports/${revision.report.id}`}>새 리포트 고유 주소 열기</Link>
  </section>;
}

function FindingCard({ finding }: { finding: Finding }) {
  return <article className={`finding severity-${finding.severity}`}>
    <div className="finding-head"><span>{finding.detail.id}</span><b>{finding.severity}</b><em>{finding.confidence}</em></div>
    <h3>{finding.detail.finding}</h3>
    <div className="comparison"><blockquote><small>원문 · {finding.detail.sourceLocation}</small>{finding.detail.statement}</blockquote><div><small>판정</small><p>{finding.detail.reasoningSummary}</p><strong>영향</strong><p>{finding.detail.impact}</p></div></div>
    <p><strong>필수 보완:</strong> {finding.detail.requiredAction}</p>
    <div className="evidence-links">{finding.detail.evidence.map((evidence) => <a key={evidence.id} href={evidence.url} target="_blank" rel="noreferrer"><ExternalLink size={13} />{evidence.id} · {evidence.sourceLocation}</a>)}</div>
  </article>;
}

function Progress({ steps, limitations }: { steps: { code: string; label: string; status: string; reason: string }[]; limitations: string[] }) {
  return <section>
    <h2>평가 진행</h2>
    <div className="progress-list">{steps.map((step) => <div key={step.code} data-status={step.status}><span /><strong>{step.label}</strong><small>{step.status}{step.reason ? ` · ${step.reason}` : ""}</small></div>)}</div>
    {limitations.length > 0 && <aside><strong>제한된 조사</strong>{limitations.map((value) => <p key={value}>{value}</p>)}</aside>}
  </section>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return <article className="metric"><small>{label}</small><strong>{value}</strong></article>;
}

function Centered({ children }: { children: React.ReactNode }) {
  return <main className="centered">{children}</main>;
}

function verdictLabel(value: string) {
  return ({
    VERDICT_EXECUTABLE: "실행 가능",
    VERDICT_CONDITIONALLY_EXECUTABLE: "조건부 실행 가능",
    VERDICT_REWRITE_REQUIRED: "재작성 필요",
    VERDICT_NOT_EXECUTABLE: "실행 불가",
  } as Record<string, string>)[value] ?? value;
}
