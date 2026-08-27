import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { SessionExpiredDialog } from "@/components/session-expired-dialog";
import { createAuthStorage } from "@/lib/auth-storage";
import { AuthStore } from "@/stores/auth/auth-store";

function Location() {
  return <span>{useLocation().pathname}</span>;
}

describe("SessionExpiredDialog", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(cleanup);

  it("shows one blocking dialog and redirects only after confirmation", () => {
    const store = new AuthStore(createAuthStorage(window.localStorage));
    store.expireSession();
    render(
      <MemoryRouter initialEntries={["/protected"]}>
        <Routes>
          <Route path="*" element={<Location />} />
        </Routes>
        <SessionExpiredDialog store={store} />
      </MemoryRouter>,
    );

    expect(screen.getAllByRole("alertdialog")).toHaveLength(1);
    expect(screen.getByText("/protected")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "확인" }));
    expect(screen.getByText("/login")).toBeInTheDocument();
    expect(store.getSnapshot().status).toBe("anonymous");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });
});
