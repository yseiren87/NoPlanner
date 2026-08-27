import { listProjects, setProjectMemberRole, updateOutputLanguage, updateProject, type Project, type ProjectRole } from "@/api/projects";

export class ProjectStore {
  state:{projects:Project[];loading:boolean;error:string|null}={projects:[],loading:false,error:null};
  listeners=new Set<()=>void>();
  subscribe=(listener:()=>void)=>{this.listeners.add(listener);return()=>this.listeners.delete(listener)};
  snapshot=()=>this.state;
  async load(){await this.run(async()=>{this.state={...this.state,projects:await listProjects()}})}
  async rename(id:string,name:string){await this.run(async()=>{const value=await updateProject(id,name);this.replace(value)})}
  async language(id:string,value:Project["outputLanguage"]){await this.run(async()=>{const project=await updateOutputLanguage(id,value);this.replace(project)})}
  async member(id:string,subject:string,role:ProjectRole){await this.run(async()=>{await setProjectMemberRole(id,subject,role)})}
  private replace(value:Project){this.state={...this.state,projects:this.state.projects.map(project=>project.id===value.id?value:project)}}
  private async run(action:()=>Promise<void>){this.state={...this.state,loading:true,error:null};this.emit();try{await action();this.state={...this.state,loading:false}}catch(error){this.state={...this.state,loading:false,error:error instanceof Error?error.message:"프로젝트 요청에 실패했습니다."}}this.emit()}
  private emit(){this.listeners.forEach(listener=>listener())}
}
export const projectStore=new ProjectStore();
