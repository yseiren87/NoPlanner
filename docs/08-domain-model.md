# 도메인 모델

## 1. 목적

이 문서는 구현 기술과 독립적으로 NoPlanner가 소유하고 관리하는 핵심 개념과 관계를 정의한다.

## 2. 핵심 개념

| 개념 | 책임 |
|---|---|
| Project | 하나의 서비스 기획과 평가 컨텍스트 관리 |
| Context | 국가·도메인·사용자·예산·일정·조직 제약 |
| Document | 입력 파일의 식별 정보와 현재 버전 연결 |
| DocumentVersion | 변경 불가능한 원문과 파싱 결과 |
| DocumentElement | 문제·목표·주장·근거·전제·요구사항·정책 |
| Dependency | 데이터·API·기술·권한·비용·협력 의존성 |
| ResearchPlan | 검증할 질문과 조사 우선순위 |
| ValidationPlan | 기획의 전제·의존성·위험에 따라 선택된 검증기와 깊이 |
| ResearchTask | 하나의 검증 질문과 실행 상태 |
| Evidence | 출처·원문 위치·내용·범위·신뢰도·확인 시점과 접근·비용·상업적 이용 조건 |
| Finding | 개별 문제와 판정·영향·심각도·보완 조건 |
| Evaluation | 특정 문서 집합과 기준 버전에 대한 평가 실행 |
| Verdict | 영역별 결과와 종합 판정 |
| Report | 평가 결과의 읽기 모델 및 내보내기 |
| Objection | 사용자의 반박과 추가 근거 |
| PlanningRun | 신규 또는 보완 기획의 생성 실행과 자체 평가 |
| DesignArtifact | 사용자 흐름·화면 명세·UI 변경안 |
| ImplementationCheck | 기획 요구사항과 코드 위치의 일치 여부 |

Intelligence는 제품의 도메인 개체를 소유하지 않는다. LLM 생성과 AI 웹 검색 공급자 호출을 표준화하는 기술 경계이며, 조사 결과·근거·판정의 최종 권한자는 아니다.

## 3. 관계

```text
Project
├── Context
├── Document*
│   └── DocumentVersion*
│       └── DocumentElement*
├── Evaluation*
│   ├── ResearchPlan
│   │   └── ResearchTask*
│   │       └── Evidence*
│   ├── ValidationPlan
│   ├── Finding*
│   │   ├── DocumentElement*
│   │   └── Evidence*
│   ├── Verdict
│   └── Report
└── Objection*
```

## 4. 주요 상태

### Evaluation

```text
DRAFT → ANALYZING → RESEARCHING → VALIDATING → REVIEWING → COMPLETED
                   ↘ NEEDS_INPUT
모든 실행 상태 → FAILED 또는 CANCELLED
```

### ResearchTask

```text
PENDING → RUNNING → VERIFIED
                  → INCONCLUSIVE
                  → ACCESS_BLOCKED
                  → FAILED
```

### Finding

```text
OPEN → PARTIALLY_RESOLVED → RESOLVED
     → DISPUTED → CONFIRMED 또는 WITHDRAWN
```

## 5. 불변 조건

- DocumentVersion의 원문은 생성 이후 변경하지 않는다.
- Finding은 하나 이상의 원문 위치 또는 `문서에 없음` 근거를 가진다.
- 외부 사실이 포함된 Finding은 하나 이상의 Evidence를 가져야 한다.
- Evidence는 확인 시점과 적용 범위를 가져야 한다.
- 데이터 의존성을 판정한 Evidence는 존재·접근·비용·상업적 이용 상태와 확인한 라이선스 또는 약관을 구분해 가져야 한다.
- 핵심 수치를 판정한 Evidence는 대상·단위·지역·기간·표본·계산 기준과 원출처 내 위치를 가져야 한다.
- 차단 Finding이 열려 있으면 실행 가능 Verdict를 만들 수 없다.
- 완료된 Evaluation은 사용한 문서·기준·모델 버전을 보존한다.
- 재평가는 기존 Evaluation을 수정하지 않고 새 Evaluation을 만든다.
- Objection은 원래 Finding과 재평가 결과를 모두 보존한다.
- ValidationPlan은 선택·미선택한 평가 영역과 이유를 모두 보존한다.
- 적용 결과는 충족·부분 충족·미충족·미검증·해당 없음·건너뜀 중 하나다.
- 핵심 전제가 미검증인 동안 Verdict는 실행 가능일 수 없다.

## 6. 식별 및 버전

- 모든 개체는 전역 고유 식별자를 사용한다.
- 사용자에게 노출하는 Finding ID는 영역 코드와 일련번호를 조합한다.
- 평가 기준, 프롬프트, 모델과 조사 커넥터 버전을 Evaluation에 고정한다.
- 동일 출처의 변경 여부를 확인할 수 있도록 Evidence에 콘텐츠 해시를 저장한다.

## 7. 논리적 책임 경계

- 문서 원본 저장과 파싱은 문서 경계가 소유한다.
- 주장과 전제 구조화의 규칙과 결과는 분석 경계가 소유하고, 필요한 LLM 실행만 Intelligence 경계에 요청한다.
- Intelligence 경계는 LLM 공급자 호출, Gemini Google Search Grounding, 공급자 응답 정규화와 비밀값 관리를 담당한다.
- 조사 경계는 검색 질문·깊이·우선순위를 결정하고, Intelligence가 반환한 출처를 직접 방문하여 근거를 정규화·검증한다.
- 문제와 판정 규칙은 평가 경계가 소유한다.
- 화면용 집계와 내보내기는 리포트 경계가 소유한다.

이 경계는 제품 개념과 책임을 구분하기 위한 것이다. 현재 구현에서는 외부 LLM·AI 검색 통신만 별도 Intelligence MSA로 분리하며, 제품 판단과 소유 데이터는 각 업무 경계에 유지한다. 그 밖의 실제 구현 경계는 개발 시점의 저장소 아키텍처 규칙에 따라 결정한다.
