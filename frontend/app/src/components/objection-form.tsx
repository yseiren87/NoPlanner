import { useState } from "react";
import { submitObjection, type Finding } from "@/api/reports";
import { Button } from "@/components/ui/button";

export function ObjectionForm({ reportId, findings }: { reportId: string; findings: Finding[] }) {
  const [findingId, setFindingId] = useState(findings[0]?.detail.id ?? "");
  const [explanation, setExplanation] = useState("");
  const [url, setURL] = useState("");
  const [status, setStatus] = useState("");

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setStatus("원출처 접근 및 내용 검증 중…");
    try {
      const result = await submitObjection(reportId, {
        findingId,
        explanation,
        evidence: [{ id: `user-${Date.now()}`, title: "사용자 추가 근거", urlOrLocation: url, applicableScope: "", contentHash: "" }],
      });
      const uncertainty = result.remainingUncertainties.length > 0 ? ` 남은 불확실성: ${result.remainingUncertainties.join(", ")}` : "";
      setStatus(`원출처 검증 ${result.verificationStatus} · 이의 제기 ${result.status}.${uncertainty}`);
      setExplanation("");
      setURL("");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "제출 실패");
    }
  };

  if (findings.length === 0) return null;
  return <section>
    <h2>이의 제기 및 추가 근거</h2>
    <p>제출한 URL의 원문 접근과 내용 해시를 먼저 확인합니다. 접수만으로 판정은 바뀌지 않으며 수정본 재평가에서 변경됩니다.</p>
    <form onSubmit={submit} className="grid gap-3 rounded-xl border p-4">
      <select value={findingId} onChange={(event) => setFindingId(event.target.value)}>{findings.map((finding) => <option key={finding.detail.id} value={finding.detail.id}>{finding.detail.id} · {finding.detail.finding}</option>)}</select>
      <textarea required className="rounded-md border p-2" value={explanation} onChange={(event) => setExplanation(event.target.value)} placeholder="반박 내용 또는 추가 설명" />
      <input required className="rounded-md border p-2" type="url" value={url} onChange={(event) => setURL(event.target.value)} placeholder="검증할 원출처 URL" />
      <Button>원출처 검증 후 제출</Button>
      {status && <p role="status">{status}</p>}
    </form>
  </section>;
}
