import { apiClient } from "@/api/client";

export type RepositoryAnalysis = { id:string;projectId:string;repositoryRoot:string;revision:string;files:{path:string;language:string;size:string;symbols:string[];contentHash:string}[];history:{revision:string;author:string;committedAt:string;subject:string}[];detectedFeatures:string[];readOnly:boolean };
export type ImplementationExpectation = { requirementId:string;statement:string;acceptanceCriteria:string;expectedPaths:string[];expectedTestPaths:string[] };
export type ImplementationReview = { id:string;evaluationId:string;repositoryRevision:string;repositoryRoot:string;projectId:string;mismatches:{id:string;requirementId:string;kind:string;planLocation:string;codeLocations:string[];finding:string;requiredChange:string;status:string}[];satisfiedRequirementIds:string[] };
export type ImplementationProposal = { id:string;reviewId:string;baseRevision:string;proposedBranch:string;patches:{path:string;unifiedDiff:string;mismatchIds:string[];requirementIds:string[]}[];workingTreeModified:boolean };
export type ConnectorCatalog = { connectors:{name:string;enabled:boolean;capabilities:string[];disabledReason:string}[] };
export type ImplementationHistory = { projectId:string;items:{id:string;kind:string;sourceId:string;resultId:string;revision:string;actorId:string;target:string;status:string;payloadHash:string;createdAt:string}[] };

async function post<T>(path:string, body:unknown):Promise<T> {
  const response = await apiClient.request(path, { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify(body) });
  if (!response.ok) throw new Error("구현 검증 요청에 실패했습니다.");
  return response.json() as Promise<T>;
}

export function analyzeRepository(projectId:string, repositoryPath:string):Promise<RepositoryAnalysis> {
  return post("/api/repositories/analyze", { projectId, repositoryPath, historyLimit:20 });
}
export function compareImplementation(evaluationId:string, repository:RepositoryAnalysis, expectations:ImplementationExpectation[]):Promise<ImplementationReview> {
  return post("/api/implementation-reviews", { evaluationId, repository, expectations });
}
export function createImplementationProposal(review:ImplementationReview):Promise<ImplementationProposal> {
  return post("/api/implementation-proposals", { review, baseRevision:review.repositoryRevision });
}
export async function getConnectors():Promise<ConnectorCatalog> {
  const response = await apiClient.request("/api/connectors");
  if (!response.ok) throw new Error("커넥터 상태를 불러오지 못했습니다.");
  return response.json() as Promise<ConnectorCatalog>;
}
export async function getImplementationHistory(projectId:string):Promise<ImplementationHistory> {
  const response = await apiClient.request(`/api/projects/${encodeURIComponent(projectId)}/implementation-history`);
  if (!response.ok) throw new Error("구현 검증 이력을 불러오지 못했습니다.");
  return response.json() as Promise<ImplementationHistory>;
}
