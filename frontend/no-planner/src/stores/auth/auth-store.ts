import { useSyncExternalStore } from "react";

import { createAuthStorage, type AuthStorage, type StoredAuthSession } from "@/lib/auth-storage";
import { tokenExpiresAfter } from "@/lib/jwt";

export type AuthSession = StoredAuthSession;

export type AuthState =
  | { status: "anonymous"; session: null }
  | { status: "authenticated"; session: AuthSession }
  | { status: "expired"; session: null };

export class AuthStore {
  private state: AuthState;
  private readonly listeners = new Set<() => void>();

  constructor(private readonly storage: AuthStorage, private readonly now: () => Date = () => new Date()) {
    const session = storage.load();
    if (session === null || !tokenExpiresAfter(session.accessToken, this.now())) {
      storage.clear();
      this.state = { status: "anonymous", session: null };
    } else {
      this.state = { status: "authenticated", session };
    }
  }

  getSnapshot = (): AuthState => this.state;

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  login(session: AuthSession) {
    if (!tokenExpiresAfter(session.accessToken, this.now())) {
      this.expireSession();
      return false;
    }
    this.storage.save(session);
    this.setState({ status: "authenticated", session });
    return true;
  }

  logout() {
    this.storage.clear();
    this.setState({ status: "anonymous", session: null });
  }

  accessToken(): string | null {
    if (this.state.status !== "authenticated") return null;
    if (!tokenExpiresAfter(this.state.session.accessToken, this.now())) {
      this.expireSession();
      return null;
    }
    return this.state.session.accessToken;
  }

  expireSession() {
    if (this.state.status === "expired") return;
    this.storage.clear();
    this.setState({ status: "expired", session: null });
  }

  acknowledgeExpiration() {
    if (this.state.status !== "expired") return;
    this.setState({ status: "anonymous", session: null });
  }

  private setState(state: AuthState) {
    this.state = state;
    this.listeners.forEach((listener) => listener());
  }
}

export const authStore = new AuthStore(createAuthStorage(window.localStorage));

export function useAuth(store: AuthStore = authStore): AuthState {
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
}
