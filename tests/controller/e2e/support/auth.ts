import type { Page } from "@playwright/test";
import { loadState } from "./state";

// Authentication helpers for the browser tests. The controller issues an opaque session
// cookie after a CSRF-protected sign-in; there is no token in the URL. These helpers share
// the page's cookie storage, so a page navigated afterwards is already authenticated.
const state = loadState();

export const url = (p: string) => `${state.baseURL}${p}`;

export async function signIn(page: Page): Promise<void> {
  // Prime the CSRF cookie (set on any GET).
  await page.request.get(url("/login"));
  const csrf =
    (await page.context().cookies()).find((c) => c.name === "sop_ctrl_csrf")?.value ?? "";
  const res = await page.request.post(url("/login"), {
    form: { token: state.token, csrf },
    maxRedirects: 0,
  });
  if (res.status() !== 303) {
    throw new Error(`sign-in failed: expected 303, got ${res.status()}`);
  }
}
