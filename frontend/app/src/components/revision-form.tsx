import { useState } from "react";
import { LoaderCircle, Upload } from "lucide-react";

export function RevisionForm({busy,onSubmit}:{busy:boolean;onSubmit:(file:File)=>Promise<void>}){
  const[file,setFile]=useState<File|null>(null);
  return <section>
    <h2>수정본 재평가</h2>
    <p>같은 문서의 새 버전으로 저장하고 변경된 영역만 비교해 판정 이력을 생성합니다.</p>
    <form onSubmit={async event=>{event.preventDefault();if(file)await onSubmit(file)}}>
      <input aria-label="수정 기획서" type="file" required onChange={event=>setFile(event.target.files?.[0]??null)}/>
      <button type="submit" disabled={busy||!file}>{busy?<LoaderCircle className="animate-spin" size={16}/>:<Upload size={16}/>}수정본 평가</button>
    </form>
  </section>
}
