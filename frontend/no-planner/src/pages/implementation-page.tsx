import { useEffect, useState, useSyncExternalStore } from "react";
import { useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { implementationStore } from "@/stores/implementation/implementation-store";

export function ImplementationPage() {
  const [params] = useSearchParams();
  const state = useSyncExternalStore(implementationStore.subscribe, implementationStore.snapshot);
  const [projectId, setProjectId] = useState(params.get("projectId") ?? "");
  const [evaluationId, setEvaluationId] = useState(params.get("evaluationId") ?? "");
  const [repositoryPath, setRepositoryPath] = useState("");
  const [requirementId, setRequirementId] = useState("REQ-001");
  const [statement, setStatement] = useState("");
  const [acceptance, setAcceptance] = useState("");
  const [codePath, setCodePath] = useState("");
  const [testPath, setTestPath] = useState("");

  useEffect(() => { if (projectId) void implementationStore.loadSupportingData(projectId); }, [projectId]);
  const busy = state.stage !== "";
  return <section className="space-y-8">
    <header><p className="eyebrow">읽기 전용 구현 검증</p><h1>기획과 실제 코드를 비교합니다.</h1><p>현재 작업 트리는 변경하지 않으며 결과는 별도 diff 제안으로 생성됩니다.</p></header>
    <form className="grid gap-3 rounded-xl border p-6" onSubmit={async (event) => { event.preventDefault(); await implementationStore.analyze(projectId, repositoryPath); }}>
      <h2>1. 저장소 분석</h2>
      <input required className="rounded-md border p-2" value={projectId} onChange={(event) => setProjectId(event.target.value)} placeholder="프로젝트 ID" />
      <input required className="rounded-md border p-2" value={repositoryPath} onChange={(event) => setRepositoryPath(event.target.value)} placeholder="로컬 Git 저장소 절대 경로" />
      <Button disabled={busy}>읽기 전용 분석</Button>
    </form>
    {state.repository && <article className="compact-card"><strong>{state.repository.repositoryRoot}</strong><p>revision {state.repository.revision} · 파일 {state.repository.files.length}개 · 변경 없음 {String(state.repository.readOnly)}</p></article>}
    {state.repository && <form className="grid gap-3 rounded-xl border p-6" onSubmit={async (event) => { event.preventDefault(); await implementationStore.compare(evaluationId, [{ requirementId, statement, acceptanceCriteria:acceptance, expectedPaths:[codePath], expectedTestPaths:[testPath] }]); }}>
      <h2>2. 요구사항과 구현 비교</h2>
      <input required className="rounded-md border p-2" value={evaluationId} onChange={(event) => setEvaluationId(event.target.value)} placeholder="평가 ID" />
      <input required className="rounded-md border p-2" value={requirementId} onChange={(event) => setRequirementId(event.target.value)} placeholder="요구사항 ID" />
      <textarea required className="rounded-md border p-2" value={statement} onChange={(event) => setStatement(event.target.value)} placeholder="요구사항" />
      <textarea required className="rounded-md border p-2" value={acceptance} onChange={(event) => setAcceptance(event.target.value)} placeholder="인수 조건" />
      <input required className="rounded-md border p-2" value={codePath} onChange={(event) => setCodePath(event.target.value)} placeholder="예상 코드 경로 또는 glob" />
      <input required className="rounded-md border p-2" value={testPath} onChange={(event) => setTestPath(event.target.value)} placeholder="예상 테스트 경로 또는 glob" />
      <Button disabled={busy}>불일치 검사</Button>
    </form>}
    {state.review && <section><h2>구현 검토 결과</h2>{state.review.mismatches.map((item) => <article className="compact-card" key={item.id}><strong>{item.kind} · {item.requirementId}</strong><p>{item.finding}</p><p>필수 변경 · {item.requiredChange}</p><small>{item.codeLocations.join(", ")}</small></article>)}<Button disabled={busy || state.review.mismatches.length === 0} onClick={() => void implementationStore.propose()}>검토용 diff 생성</Button></section>}
    {state.proposal && <section><h2>변경안 · {state.proposal.proposedBranch}</h2><p>현재 작업 트리 변경: {String(state.proposal.workingTreeModified)}</p>{state.proposal.patches.map((patch) => <article key={patch.path}><strong>{patch.path}</strong><pre className="overflow-auto rounded-md border p-3 text-xs">{patch.unifiedDiff}</pre></article>)}</section>}
    {state.connectors && <section><h2>외부 연동 상태</h2>{state.connectors.connectors.map((connector) => <p key={connector.name}>{connector.name} · {connector.enabled?"사용 가능":`비활성: ${connector.disabledReason}`}</p>)}</section>}
    {state.history && <section><h2>감사 이력</h2>{state.history.items.map((item) => <p key={item.id}>{item.kind} · {item.status} · {item.revision}</p>)}</section>}
    {state.stage && <p role="status">{state.stage}</p>}{state.error && <p role="alert">{state.error}</p>}
  </section>;
}
