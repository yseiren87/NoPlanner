import { useState } from "react";
import { beginGoogleLogin } from "@/api/auth";
import { Button } from "@/components/ui/button";
export function LoginPage() {
  const [error,setError]=useState("");
  const login=async()=>{try{const result=await beginGoogleLogin();window.location.assign(result.authorizationUrl)}catch(e){setError(e instanceof Error?e.message:"로그인 실패")}};
  return (
    <main className="grid min-h-screen place-items-center bg-background px-6 text-foreground">
      <section className="w-full max-w-sm space-y-3 text-center">
        <p className="text-sm font-semibold text-primary">NoPlanner</p>
        <h1 className="text-3xl font-bold tracking-tight">로그인이 필요합니다.</h1>
        <p className="text-sm leading-6 text-muted-foreground">Google 계정으로 로그인해 기획 검증을 시작하세요.</p>
        <Button className="w-full" onClick={()=>void login()}>Google로 로그인</Button>
        {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
      </section>
    </main>
  );
}
