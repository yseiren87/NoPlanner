package report

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

func Export(value Report, format string) ([]byte, string, string, error) {
	markdown := Markdown(value)
	switch format {
	case "markdown":
		return []byte(markdown), "text/markdown; charset=utf-8", value.ID + ".md", nil
	case "json":
		content, err := json.MarshalIndent(value, "", "  ")
		return content, "application/json", value.ID + ".json", err
	case "pdf":
		return simplePDF(markdown), "application/pdf", value.ID + ".pdf", nil
	case "docx":
		content, err := docx(value)
		return content, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", value.ID + ".docx", err
	default:
		return nil, "", "", ErrInvalid
	}
}

func Markdown(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 통합 평가 리포트\n\n- 리포트 ID: `%s`\n- 평가 ID: `%s`\n- 대상 문서: %s v%d\n- 국가 / 도메인 / 사용자: %s / %s / %s\n- 평가 기준 / 모델: %s / %s\n\n## 종합 판정\n\n**%s** — %s\n\n구현 시작 가능: %t  \n판정 신뢰도: %s\n\n", r.ProjectName, r.ID, r.EvaluationID, r.DocumentName, r.DocumentVersion, r.Country, r.Domain, r.TargetUser, r.CriteriaVersion, r.Model, r.Verdict, r.Conclusion, r.ImplementationMayStart, r.Confidence)
	b.WriteString("## 기획 재구성\n\n")
	for _, row := range [][2]string{{"문제", r.Summary.Problem}, {"목적", r.Summary.Purpose}, {"대상 사용자", r.Summary.TargetUser}, {"환경", r.Summary.Environment}, {"해결책", r.Summary.Solution}, {"성공 기준", r.Summary.SuccessCriteria}} {
		fmt.Fprintf(&b, "- %s: %s\n", row[0], defined(row[1]))
	}
	b.WriteString("\n## 핵심 문제 및 전체 문제\n\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "### [%s] %s\n\n- 심각도 / 신뢰도: %s / %s\n- 영역 / 유형: %s / %s\n- 원문: %s — %s\n- 판정: %s\n- 근거: ", f.Detail.ID, f.Detail.Finding, f.Severity, f.Confidence, f.Detail.Area, f.Detail.ProblemType, f.Detail.SourceLocation, f.Detail.Statement, f.Detail.ReasoningSummary)
		for _, e := range f.Detail.Evidence {
			fmt.Fprintf(&b, "[%s](%s) (%s), ", e.ID, e.URL, e.SourceLocation)
		}
		fmt.Fprintf(&b, "\n- 영향: %s\n- 필수 보완: %s\n- 검증 방법: %s\n\n", f.Detail.Impact, f.Detail.RequiredAction, f.Detail.VerificationMethod)
	}
	b.WriteString("## 영역별 평가\n\n| 영역 | 상태 | 문제 | 신뢰도 | 이유 |\n|---|---|---:|---|---|\n")
	for _, a := range r.Areas {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s |\n", a.Area, a.Status, a.FindingCount, a.Confidence, a.Reason)
	}
	b.WriteString("\n## 전제 및 의존성\n\n")
	for _, a := range r.Assumptions {
		fmt.Fprintf(&b, "- [%s] %s — %s; 의존성: %s; 근거: %s\n", a.ID, a.Statement, a.Status, strings.Join(a.Dependencies, ", "), strings.Join(a.EvidenceIDs, ", "))
	}
	b.WriteString("\n## 내부 상충\n\n")
	for _, c := range r.Contradictions {
		fmt.Fprintf(&b, "- [%s] %s `%s` ↔ %s `%s`: %s\n", c.ID, c.FirstLocation, c.FirstStatement, c.SecondLocation, c.SecondStatement, c.Reason)
	}
	b.WriteString("\n## 조사 결과\n\n")
	for _, q := range r.Research {
		fmt.Fprintf(&b, "- 질문: %s\n  - 결과: %s\n", q.Question, q.Conclusion)
		for _, e := range q.Sources {
			fmt.Fprintf(&b, "  - 원출처 [%s](%s) — %s\n", e.ID, e.URL, e.SourceLocation)
		}
		for _, e := range q.Conflicts {
			fmt.Fprintf(&b, "  - 상충 근거 [%s](%s)\n", e.ID, e.URL)
		}
		if len(q.Uncertainties) > 0 {
			fmt.Fprintf(&b, "  - 불확실성: %s\n", strings.Join(q.Uncertainties, ", "))
		}
	}
	b.WriteString("\n## 보완 및 최종 권고\n\n")
	for _, v := range r.RequiredActions {
		fmt.Fprintf(&b, "- 필수: %s\n", v)
	}
	for _, v := range r.Recommendations {
		fmt.Fprintf(&b, "- 개선: %s\n", v)
	}
	for _, v := range r.VerdictChangeConditions {
		fmt.Fprintf(&b, "- 판정 변경 조건: %s\n", v)
	}
	b.WriteString("\n## 평가 한계\n\n")
	for _, v := range r.Limitations {
		fmt.Fprintf(&b, "- %s\n", v)
	}
	return b.String()
}
func defined(value string) string {
	if strings.TrimSpace(value) == "" {
		return "정의되지 않음"
	}
	return value
}

func simplePDF(text string) []byte {
	safe := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)", "\n", " ").Replace(text)
	if len(safe) > 4000 {
		safe = safe[:4000]
	}
	stream := fmt.Sprintf("BT /F1 8 Tf 40 800 Td (%s) Tj ET", safe)
	parts := []string{"%PDF-1.4\n", "1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n", "2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n", "3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >> endobj\n", "4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n", fmt.Sprintf("5 0 obj << /Length %d >> stream\n%s\nendstream endobj\n", len(stream), stream)}
	var b bytes.Buffer
	offsets := []int{0}
	for _, p := range parts {
		offsets = append(offsets, b.Len())
		b.WriteString(p)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer << /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xref)
	return b.Bytes()
}
func docx(r Report) ([]byte, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`, `_rels/.rels`: `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`, `word/document.xml`: `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t xml:space="preserve">` + html.EscapeString(Markdown(r)) + `</w:t></w:r></w:p></w:body></w:document>`}
	for name, content := range files {
		w, err := z.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
