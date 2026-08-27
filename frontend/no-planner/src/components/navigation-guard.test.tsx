import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { NavigationGuard } from "@/components/navigation-guard";
import { createAuthStorage } from "@/lib/auth-storage";
import { AuthStore } from "@/stores/auth/auth-store";

function renderRoutes(store: AuthStore) {
  render(
    <MemoryRouter initialEntries={["/protected"]}>
      <Routes>
        <Route path="/login" element={<div>login screen</div>} />
        <Route element={<NavigationGuard store={store} />}>
          <Route path="/protected" element={<div>sensitive protected screen</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("NavigationGuard", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(cleanup);

  it("redirects an anonymous user without rendering protected content", () => {
    renderRoutes(new AuthStore(createAuthStorage(window.localStorage)));
    expect(screen.getByText("login screen")).toBeInTheDocument();
    expect(screen.queryByText("sensitive protected screen")).not.toBeInTheDocument();
  });

  it("redirects an expired session without rendering protected content", () => {
    const storage = createAuthStorage(window.localStorage);
    storage.save({
      accessToken: `header.${btoa(JSON.stringify({ exp: 1_600_000_000 }))}.signature`,
      user: { subject: "user", email: "user@example.com", name: "User", pictureUrl: "" },
    });
    renderRoutes(new AuthStore(storage, () => new Date(1_700_000_000_000)));
    expect(screen.getByText("login screen")).toBeInTheDocument();
    expect(screen.queryByText("sensitive protected screen")).not.toBeInTheDocument();
  });

  it("renders a protected route for a token with a future exp", () => {
    const storage = createAuthStorage(window.localStorage);
    storage.save({
      accessToken: `header.${btoa(JSON.stringify({ exp: 1_800_000_000 }))}.signature`,
      user: { subject: "user", email: "user@example.com", name: "User", pictureUrl: "" },
    });
    renderRoutes(new AuthStore(storage, () => new Date(1_700_000_000_000)));
    expect(screen.getByText("sensitive protected screen")).toBeInTheDocument();
  });
});
