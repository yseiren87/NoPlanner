import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { type AuthStore, authStore, useAuth } from "@/stores/auth/auth-store";

type SessionExpiredDialogProps = {
  store?: AuthStore;
};

export function SessionExpiredDialog({ store = authStore }: SessionExpiredDialogProps) {
  const auth = useAuth(store);
  const navigate = useNavigate();
  const open = auth.status === "expired";

  function confirm() {
    store.acknowledgeExpiration();
    navigate("/login", { replace: true });
  }

  return (
    <Dialog open={open}>
      <DialogContent
        role="alertdialog"
        aria-describedby="session-expired-description"
        onEscapeKeyDown={(event) => event.preventDefault()}
        onPointerDownOutside={(event) => event.preventDefault()}
      >
        <DialogTitle>로그인이 풀렸습니다.</DialogTitle>
        <DialogDescription id="session-expired-description">
          세션이 만료되었거나 서버에서 인증을 확인할 수 없습니다. 다시 로그인해 주세요.
        </DialogDescription>
        <div className="mt-6 flex justify-end">
          <Button onClick={confirm}>확인</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
