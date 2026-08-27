import { beforeEach, describe, expect, it, vi } from "vitest";

import { createAuthStorage } from "@/lib/auth-storage";
import { AuthStore, type AuthSession } from "@/stores/auth/auth-store";

const session: AuthSession = {
  accessToken: `header.${btoa(JSON.stringify({ exp: 1_800_000_000 }))}.signature`,
  user: {
    subject: "google-subject",
    email: "developer@example.com",
    name: "Developer",
    pictureUrl: "https://example.com/profile.png",
  },
};

describe("AuthStore", () => {
  beforeEach(() => window.localStorage.clear());

  it("persists a login and restores it in a new store", () => {
    const storage = createAuthStorage(window.localStorage);
    const store = new AuthStore(storage, () => new Date(1_700_000_000_000));

    store.login(session);

    expect(store.getSnapshot()).toEqual({ status: "authenticated", session });
    expect(new AuthStore(storage, () => new Date(1_700_000_000_000)).getSnapshot()).toEqual({ status: "authenticated", session });
  });

  it("removes the persisted session on logout", () => {
    const storage = createAuthStorage(window.localStorage);
    const store = new AuthStore(storage, () => new Date(1_700_000_000_000));
    store.login(session);

    store.logout();

    expect(store.getSnapshot()).toEqual({ status: "anonymous", session: null });
    expect(new AuthStore(storage).getSnapshot()).toEqual({ status: "anonymous", session: null });
  });

  it("ignores malformed persisted values", () => {
    window.localStorage.setItem("noplanner.auth.session", "not-json");
    expect(new AuthStore(createAuthStorage(window.localStorage)).getSnapshot()).toEqual({
      status: "anonymous",
      session: null,
    });
  });

  it("notifies subscribers when login and logout change state", () => {
    const store = new AuthStore(createAuthStorage(window.localStorage), () => new Date(1_700_000_000_000));
    const listener = vi.fn();
    const unsubscribe = store.subscribe(listener);

    store.login(session);
    store.logout();
    unsubscribe();
    store.login(session);

    expect(listener).toHaveBeenCalledTimes(2);
  });

  it("rejects and removes tokens with no exp or an expired exp", () => {
    const storage = createAuthStorage(window.localStorage);
    storage.save({ ...session, accessToken: `header.${btoa(JSON.stringify({ sub: "user" }))}.signature` });
    expect(new AuthStore(storage).getSnapshot().status).toBe("anonymous");
    expect(storage.load()).toBeNull();

    storage.save({ ...session, accessToken: `header.${btoa(JSON.stringify({ exp: 1_600_000_000 }))}.signature` });
    expect(new AuthStore(storage, () => new Date(1_700_000_000_000)).getSnapshot().status).toBe("anonymous");
    expect(storage.load()).toBeNull();
  });

  it("does not notify twice when concurrent failures expire the same session", () => {
    const store = new AuthStore(createAuthStorage(window.localStorage), () => new Date(1_700_000_000_000));
    store.login(session);
    const listener = vi.fn();
    store.subscribe(listener);

    store.expireSession();
    store.expireSession();

    expect(store.getSnapshot().status).toBe("expired");
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
