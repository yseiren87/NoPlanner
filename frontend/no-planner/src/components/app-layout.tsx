import { Link, Outlet } from "react-router-dom";

export function AppLayout() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border bg-background/90 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <Link to="/" className="text-lg font-bold tracking-tight">NoPlanner</Link>
          <nav className="flex items-center gap-4 text-sm"><Link to="/">기획 평가</Link><Link to="/projects">프로젝트</Link><Link to="/planning/new">자율 기획</Link><Link to="/implementation">구현 검증</Link><span className="text-xs text-muted-foreground">v{__APP_VERSION__}</span></nav>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-6 py-12">
        <Outlet />
      </main>
    </div>
  );
}
