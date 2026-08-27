import { Navigate, Outlet, useLocation } from "react-router-dom";

import { authStore, type AuthStore, useAuth } from "@/stores/auth/auth-store";

type NavigationGuardProps = {
  store?: AuthStore;
};

export function NavigationGuard({ store = authStore }: NavigationGuardProps) {
  const auth = useAuth(store);
  const location = useLocation();

  if (auth.status === "expired") return null;

  if (auth.status !== "authenticated") {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }

  return <Outlet />;
}
