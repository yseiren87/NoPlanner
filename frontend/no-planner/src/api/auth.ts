export interface LoginStart { authorizationUrl: string }
export interface LoginResult { accessToken: string; expiresAt: string; user: { subject:string; email:string; name:string; pictureUrl:string } }
export async function beginGoogleLogin():Promise<LoginStart>{const response=await fetch("/api/auth/google");if(!response.ok)throw new Error("Google 로그인을 시작하지 못했습니다.");return response.json() as Promise<LoginStart>}
export async function completeGoogleLogin(code:string,state:string):Promise<LoginResult>{const response=await fetch("/api/auth/google/callback",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({code,state})});if(!response.ok)throw new Error("Google 로그인에 실패했습니다.");return response.json() as Promise<LoginResult>}
