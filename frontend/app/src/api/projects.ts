import { apiClient } from "@/api/client";

export type ProjectRole = "PROJECT_ROLE_OWNER"|"PROJECT_ROLE_EDITOR"|"PROJECT_ROLE_VIEWER";
export type Project = { id:string;name:string;createdAt:string;updatedAt:string;callerRole:ProjectRole;outputLanguage:"OUTPUT_LANGUAGE_KOREAN"|"OUTPUT_LANGUAGE_ENGLISH" };

async function request<T>(path:string, init?:RequestInit):Promise<T> {
  const response = await apiClient.request(path, init);
  if (!response.ok) throw new Error("프로젝트 요청에 실패했습니다.");
  return response.json() as Promise<T>;
}
export async function listProjects():Promise<Project[]> { return (await request<{projects:Project[]}>("/api/projects")).projects??[]; }
export function updateProject(id:string,name:string):Promise<Project> { return request(`/api/projects/${encodeURIComponent(id)}`, {method:"PATCH",headers:{"Content-Type":"application/json"},body:JSON.stringify({name})}); }
export function updateOutputLanguage(id:string,outputLanguage:Project["outputLanguage"]):Promise<Project> { return request(`/api/projects/${encodeURIComponent(id)}/output-language`, {method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify({outputLanguage})}); }
export function setProjectMemberRole(id:string,subject:string,role:ProjectRole):Promise<{projectId:string;subject:string;role:ProjectRole}> { return request(`/api/projects/${encodeURIComponent(id)}/members/${encodeURIComponent(subject)}`, {method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify({role})}); }
