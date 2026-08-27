import { describe, expect, it } from "vitest";

import indexHtml from "../index.html?raw";

describe("browser security policy", () => {
  it("blocks inline scripts, plugins, base rewriting, and external connections", () => {
    const document = new DOMParser().parseFromString(indexHtml, "text/html");
    const policy = document.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute("content") ?? "";

    expect(policy).toContain("default-src 'self'");
    expect(policy).toContain("base-uri 'none'");
    expect(policy).toContain("object-src 'none'");
    expect(policy).toContain("script-src 'self'");
    expect(policy).toContain("connect-src 'self'");
    expect(policy).not.toContain("script-src 'unsafe-inline'");
  });

  it("does not send referrer data to external pages", () => {
    const document = new DOMParser().parseFromString(indexHtml, "text/html");
    expect(document.querySelector('meta[name="referrer"]')?.getAttribute("content")).toBe("no-referrer");
  });
});
