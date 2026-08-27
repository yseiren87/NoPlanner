import { analyzeRepository, compareImplementation, createImplementationProposal, getConnectors, getImplementationHistory, type ConnectorCatalog, type ImplementationExpectation, type ImplementationHistory, type ImplementationProposal, type ImplementationReview, type RepositoryAnalysis } from "@/api/implementation";

export type ImplementationState = { repository:RepositoryAnalysis|null;review:ImplementationReview|null;proposal:ImplementationProposal|null;connectors:ConnectorCatalog|null;history:ImplementationHistory|null;stage:string;error:string|null };

export class ImplementationStore {
  state:ImplementationState = { repository:null, review:null, proposal:null, connectors:null, history:null, stage:"", error:null };
  listeners = new Set<()=>void>();
  subscribe = (listener:()=>void) => { this.listeners.add(listener); return () => this.listeners.delete(listener); };
  snapshot = () => this.state;

  async analyze(projectId:string, repositoryPath:string) {
    await this.run("저장소 분석 중", async () => { this.state = { ...this.state, repository:await analyzeRepository(projectId, repositoryPath), review:null, proposal:null }; });
  }
  async compare(evaluationId:string, expectations:ImplementationExpectation[]) {
    if (!this.state.repository) return;
    await this.run("기획과 구현 비교 중", async () => { this.state = { ...this.state, review:await compareImplementation(evaluationId, this.state.repository!, expectations), proposal:null }; });
  }
  async propose() {
    if (!this.state.review) return;
    await this.run("검토용 변경안 생성 중", async () => { this.state = { ...this.state, proposal:await createImplementationProposal(this.state.review!) }; });
  }
  async loadSupportingData(projectId:string) {
    await this.run("이력과 커넥터 확인 중", async () => {
      const [connectors, history] = await Promise.all([getConnectors(), getImplementationHistory(projectId)]);
      this.state = { ...this.state, connectors, history };
    });
  }
  private async run(stage:string, action:()=>Promise<void>) {
    this.state = { ...this.state, stage, error:null }; this.emit();
    try { await action(); this.state = { ...this.state, stage:"" }; }
    catch (error) { this.state = { ...this.state, stage:"", error:error instanceof Error?error.message:"구현 검증에 실패했습니다." }; }
    this.emit();
  }
  private emit() { this.listeners.forEach((listener) => listener()); }
}

export const implementationStore = new ImplementationStore();
