import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { completeGoogleLogin } from "@/api/auth";
import { authStore } from "@/stores/auth/auth-store";
export function AuthCallbackPage(){const [params]=useSearchParams();const navigate=useNavigate();const [error,setError]=useState("");useEffect(()=>{const code=params.get("code"),state=params.get("state");if(!code||!state){setError("OAuth 응답이 올바르지 않습니다.");return}void completeGoogleLogin(code,state).then(v=>{authStore.login({accessToken:v.accessToken,user:v.user});navigate("/",{replace:true})}).catch(e=>setError(e instanceof Error?e.message:"로그인 실패"))},[navigate,params]);return <main className="grid min-h-screen place-items-center"><p>{error||"Google 로그인을 완료하는 중입니다…"}</p></main>}
