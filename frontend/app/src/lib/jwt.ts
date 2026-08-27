export function tokenExpiresAfter(token: string, now: Date): boolean {
  const parts = token.split(".");
  if (parts.length !== 3) return false;

  try {
    const payload: unknown = JSON.parse(decodeBase64Url(parts[1]));
    if (typeof payload !== "object" || payload === null) return false;
    const { exp } = payload as { exp?: unknown };
    return typeof exp === "number" && Number.isFinite(exp) && exp > Math.floor(now.getTime() / 1000);
  } catch {
    return false;
  }
}

function decodeBase64Url(value: string): string {
  const base64 = value.replaceAll("-", "+").replaceAll("_", "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
  return atob(base64);
}
