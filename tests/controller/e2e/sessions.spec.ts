import { expect, request, test, type APIRequestContext } from "@playwright/test";
import type { ChildProcess } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { loadState, WORK_DIR } from "./support/state";
import {
  freePort,
  randomToken,
  startController,
  stopController,
  waitForHealth,
} from "./support/fixtures";

// Remembered-device sessions. A successful token login mints an opaque session cookie; a
// returning device is authenticated by that cookie alone (never the token) until the
// session expires or is revoked. These run against a *dedicated* controller on its own
// port and its own session file, so restart and token-rotation coverage never disturbs the
// shared server. Sessions are only ever validated server-side; the token is never stored
// in the cookie or browser storage.
const state = loadState();
const roots = Object.values(state.projects).map((p) => p.root);
const readonly = state.projects.readonly;

// signIn performs the CSRF-protected sign-in on one context and returns the HTTP status.
async function signIn(
  ctx: APIRequestContext,
  base: string,
  token: string,
): Promise<number> {
  await ctx.get(`${base}/login`);
  const csrf =
    (await ctx.storageState()).cookies.find((c) => c.name === "sop_ctrl_csrf")?.value ??
    "";
  const res = await ctx.post(`${base}/login`, {
    form: { token, csrf },
    maxRedirects: 0,
  });
  return res.status();
}

test.describe("remembered device sessions", () => {
  // Serial: each test builds on the state the previous one left (the same remembered
  // device, a restart, a rotation).
  test.describe.configure({ mode: "serial" });

  const sessionFile = path.join(WORK_DIR, "sessions-dedicated.json");
  let base = "";
  let token = randomToken();
  let child: ChildProcess | undefined;
  let device: APIRequestContext;

  // boot (re)starts the dedicated controller with the given token. A fresh port each time
  // keeps restarts off a lingering socket; cookies are host-scoped, not port-scoped, so the
  // remembered device keeps working across the move.
  async function boot(withToken: string): Promise<void> {
    token = withToken;
    const port = await freePort();
    base = `http://127.0.0.1:${port}`;
    child = startController(state.bin, roots, port, withToken, sessionFile);
    await waitForHealth(base);
  }

  test.beforeAll(async () => {
    fs.rmSync(sessionFile, { force: true });
    await boot(token);
    device = await request.newContext();
  });

  test.afterAll(async () => {
    await device?.dispose();
    if (child) await stopController(child);
    fs.rmSync(sessionFile, { force: true });
  });

  test("1. a first-time device must present the token", async () => {
    // No cookie yet: the inbox is not reachable.
    expect((await device.get(`${base}/approvals`, { maxRedirects: 0 })).status()).toBe(401);
    // A wrong token is rejected (rendered page, no session).
    expect(await signIn(device, base, "not-the-token")).toBe(200);
    // The right token opens a session (303 redirect to the app).
    expect(await signIn(device, base, token)).toBe(303);
  });

  test("2. the remembered device reopens without the token", async () => {
    expect((await device.get(`${base}/approvals`, { maxRedirects: 0 })).status()).toBe(200);
    // /login sends an authenticated device on rather than showing the token form.
    expect((await device.get(`${base}/login`, { maxRedirects: 0 })).status()).toBe(303);
  });

  test("2b. an authenticated session still requires CSRF", async () => {
    const res = await device.post(
      `${base}/projects/${readonly.id}/tasks/${readonly.task}/commands/approve`,
      { maxRedirects: 0 },
    );
    expect(res.status()).toBe(403);
  });

  test("3. a new browser context is not authenticated", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto(`${base}/approvals`);
    await page.waitForURL((u) => u.pathname === "/login");
    await ctx.close();
  });

  test("4. a session survives a controller restart", async () => {
    if (child) await stopController(child);
    await boot(token);
    expect((await device.get(`${base}/approvals`, { maxRedirects: 0 })).status()).toBe(200);
  });

  test("5. rotating the token invalidates remembered sessions", async () => {
    if (child) await stopController(child);
    await boot(randomToken());
    // The remembered (pre-rotation) device is no longer trusted.
    expect((await device.get(`${base}/approvals`, { maxRedirects: 0 })).status()).toBe(401);
    // A fresh device signs in with the new token.
    const fresh = await request.newContext();
    expect(await signIn(fresh, base, token)).toBe(303);
    await fresh.dispose();
  });

  test("6. the session cookie is HttpOnly, SameSite, and lasts up to 30 days", async () => {
    const ctx = await request.newContext();
    expect(await signIn(ctx, base, token)).toBe(303);
    const cookie = (await ctx.storageState()).cookies.find(
      (c) => c.name === "sop_ctrl_session",
    );
    expect(cookie, "a session cookie must be set").toBeTruthy();
    expect(cookie!.httpOnly).toBe(true);
    expect(cookie!.sameSite).toBe("Lax");
    expect(cookie!.value).not.toContain(token);
    // Max-Age is capped at 30 days.
    const days = (cookie!.expires - Date.now() / 1000) / 86400;
    expect(days).toBeGreaterThan(29);
    expect(days).toBeLessThanOrEqual(30.1);
    await ctx.dispose();
  });

  test("7. behind the HTTPS proxy the session cookie is Secure", async () => {
    const ctx = await request.newContext();
    await ctx.get(`${base}/login`);
    const csrf =
      (await ctx.storageState()).cookies.find((c) => c.name === "sop_ctrl_csrf")?.value ?? "";
    const res = await ctx.post(`${base}/login`, {
      form: { token, csrf },
      // The peer is loopback, so the proxy header is honored for the Secure flag.
      headers: { "X-Forwarded-Proto": "https" },
      maxRedirects: 0,
    });
    const setCookie = res
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value)
      .join("\n");
    const session =
      setCookie.split("\n").find((l) => l.startsWith("sop_ctrl_session=")) ?? "";
    expect(session).toMatch(/HttpOnly/i);
    expect(session).toMatch(/Secure/i);
    expect(session).toMatch(/SameSite=Lax/i);
    await ctx.dispose();
  });
});
