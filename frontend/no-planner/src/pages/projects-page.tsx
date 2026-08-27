import { useEffect, useState, useSyncExternalStore } from "react";
import { Link } from "react-router-dom";
import type { ProjectRole } from "@/api/projects";
import { Button } from "@/components/ui/button";
import { projectStore } from "@/stores/project/project-store";

export function ProjectsPage(){
  const state=useSyncExternalStore(projectStore.subscribe,projectStore.snapshot);
  useEffect(()=>{void projectStore.load()},[]);
  return <section className="space-y-6"><header><p className="eyebrow">프로젝트와 접근 역할</p><h1>프로젝트 관리</h1></header>{state.projects.map(project=><ProjectCard key={project.id} project={project}/>)}{!state.loading&&state.projects.length===0&&<p>참여 중인 프로젝트가 없습니다. 기획서를 평가하면 프로젝트가 생성됩니다.</p>}{state.loading&&<p role="status">처리 중…</p>}{state.error&&<p role="alert">{state.error}</p>}</section>
}
function ProjectCard({project}:{project:ReturnType<typeof projectStore.snapshot>["projects"][number]}){
  const[name,setName]=useState(project.name);const[subject,setSubject]=useState("");const[role,setRole]=useState<ProjectRole>("PROJECT_ROLE_VIEWER");const owner=project.callerRole==="PROJECT_ROLE_OWNER";
  return <article className="grid gap-3 rounded-xl border p-6"><small>{project.id} · {project.callerRole}</small><form className="flex gap-2" onSubmit={async event=>{event.preventDefault();await projectStore.rename(project.id,name)}}><input className="grow rounded-md border p-2" value={name} onChange={event=>setName(event.target.value)}/><Button disabled={project.callerRole==="PROJECT_ROLE_VIEWER"}>이름 저장</Button></form><label>결과 언어 <select value={project.outputLanguage} disabled={project.callerRole==="PROJECT_ROLE_VIEWER"} onChange={event=>void projectStore.language(project.id,event.target.value as typeof project.outputLanguage)}><option value="OUTPUT_LANGUAGE_KOREAN">한국어</option><option value="OUTPUT_LANGUAGE_ENGLISH">English</option></select></label>{owner&&<form className="flex flex-wrap gap-2" onSubmit={async event=>{event.preventDefault();await projectStore.member(project.id,subject,role);setSubject("")}}><input required className="grow rounded-md border p-2" value={subject} onChange={event=>setSubject(event.target.value)} placeholder="Google 사용자 subject"/><select value={role} onChange={event=>setRole(event.target.value as ProjectRole)}><option value="PROJECT_ROLE_OWNER">소유자</option><option value="PROJECT_ROLE_EDITOR">편집자</option><option value="PROJECT_ROLE_VIEWER">열람자</option></select><Button>역할 저장</Button></form>}<Link to={`/implementation?projectId=${encodeURIComponent(project.id)}`}>구현 검증 열기</Link></article>
}
