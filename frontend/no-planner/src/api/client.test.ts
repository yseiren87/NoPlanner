import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClient, AuthenticationRequiredError, UntrustedApiOriginError } from "@/api/client";
import { createAuthStorage } from "@/lib/auth-storage";
import { AuthStore, type AuthSession } from "@/stores/auth/auth-store";

const session: AuthSession = {
  accessToken: `header.${btoa(JSON.stringify({ exp: 1_800_000_000 }))}.signature`,
  user: { subject: "user", email: "user@example.com", name: "User", pictureUrl: "" },
};

function authenticatedStore() {
  const store = new AuthStore(createAuthStorage(window.localStorage), () => new Date(1_700_000_000_000));
  store.login(session);
  return store;
}

describe("ApiClient", () => {
  beforeEach(() => window.localStorage.clear());

  it("adds the bearer token to every protected request", async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));
    const client = new ApiClient(authenticatedStore(), transport);

    await client.request("/api/projects", { headers: { "X-Request-ID": "request-1" } });

    const headers = new Headers(transport.mock.calls[0][1]?.headers);
    expect(headers.get("Authorization")).toBe(`Bearer ${session.accessToken}`);
    expect(headers.get("X-Request-ID")).toBe("request-1");
  });

  it("does not send a JWT to an external origin", async () => {
    const transport = vi.fn<typeof fetch>();
    const client = new ApiClient(authenticatedStore(), transport);

    await expect(client.request("https://attacker.example/collect")).rejects.toBeInstanceOf(UntrustedApiOriginError);

    expect(transport).not.toHaveBeenCalled();
  });

  it("turns simultaneous 401 responses into one expiration transition", async () => {
    const store = authenticatedStore();
    const listener = vi.fn();
    store.subscribe(listener);
    const transport = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 401 }));
    const client = new ApiClient(store, transport);

    const results = await Promise.allSettled([client.request("/api/one"), client.request("/api/two")]);

    expect(results.every((result) => result.status === "rejected" && result.reason instanceof AuthenticationRequiredError)).toBe(true);
    expect(store.getSnapshot().status).toBe("expired");
    expect(listener).toHaveBeenCalledTimes(1);
    expect(window.localStorage.length).toBe(0);
  });

  it("blocks new requests after session expiration", async () => {
    const store = authenticatedStore();
    store.expireSession();
    const transport = vi.fn<typeof fetch>();
    const client = new ApiClient(store, transport);

    await expect(client.request("/api/projects")).rejects.toBeInstanceOf(AuthenticationRequiredError);
    expect(transport).not.toHaveBeenCalled();
  });

  it("detects expiration before sending the request", async () => {
    let now = new Date(1_700_000_000_000);
    const store = new AuthStore(createAuthStorage(window.localStorage), () => now);
    store.login(session);
    now = new Date(1_900_000_000_000);
    const transport = vi.fn<typeof fetch>();

    await expect(new ApiClient(store, transport).request("/api/projects")).rejects.toBeInstanceOf(AuthenticationRequiredError);
    expect(store.getSnapshot().status).toBe("expired");
    expect(transport).not.toHaveBeenCalled();
  });
});
