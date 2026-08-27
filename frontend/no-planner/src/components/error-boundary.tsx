import { Component, type ErrorInfo, type ReactNode } from "react";

import { Button } from "@/components/ui/button";

type Props = { children: ReactNode };
type State = { hasError: boolean };

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Unhandled application error", error, info.componentStack);
  }

  render() {
    if (!this.state.hasError) return this.props.children;

    return (
      <main className="grid min-h-screen place-items-center bg-background px-6 text-foreground">
        <section className="max-w-md space-y-4 text-center">
          <p className="text-sm font-semibold text-destructive">화면을 표시하지 못했습니다</p>
          <h1 className="text-2xl font-bold">예상하지 못한 오류가 발생했습니다.</h1>
          <p className="text-sm text-muted-foreground">새로고침한 뒤에도 문제가 계속되면 잠시 후 다시 시도해 주세요.</p>
          <Button onClick={() => window.location.reload()}>새로고침</Button>
        </section>
      </main>
    );
  }
}
