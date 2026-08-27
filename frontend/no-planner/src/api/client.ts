import { type AuthStore, authStore } from "@/stores/auth/auth-store";

export class AuthenticationRequiredError extends Error {
  constructor() {
    super("authentication required");
    this.name = "AuthenticationRequiredError";
  }
}

export class UntrustedApiOriginError extends Error {
  constructor() {
    super("protected API requests must use the application origin");
    this.name = "UntrustedApiOriginError";
  }
}

export class ApiClient {
  constructor(
    private readonly auth: AuthStore = authStore,
    private readonly transport: typeof fetch = fetch,
  ) {}

  async request(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
    const accessToken = this.auth.accessToken();
    if (accessToken === null) throw new AuthenticationRequiredError();

    const requestUrl = new URL(input instanceof Request ? input.url : input.toString(), window.location.href);
    if (requestUrl.origin !== window.location.origin) throw new UntrustedApiOriginError();

    const headers = new Headers(init.headers);
    headers.set("Authorization", `Bearer ${accessToken}`);
    const response = await this.transport(input, { ...init, headers });
    if (response.status === 401) {
      this.auth.expireSession();
      throw new AuthenticationRequiredError();
    }
    return response;
  }
}

export const apiClient = new ApiClient();
