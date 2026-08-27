import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";

export function NotFoundPage() {
  return (
    <section className="py-20 text-center">
      <p className="text-sm font-semibold text-primary">404</p>
      <h1 className="mt-2 text-3xl font-bold">페이지를 찾을 수 없습니다.</h1>
      <Button asChild className="mt-6">
        <Link to="/">홈으로 돌아가기</Link>
      </Button>
    </section>
  );
}
