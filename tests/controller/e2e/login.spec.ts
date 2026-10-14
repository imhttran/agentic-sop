import { expect, test } from '@playwright/test';
import { loadState } from './support/state';

// Token login over the controller's own routes. The session is an opaque cookie; the
// token is submitted once in a form body and never appears in a URL, the page, or browser
// storage. Session expiry and token rotation need a short TTL / a restart, so they are
// covered deterministically by the Go tests (TestSessionExpiry,
// TestTokenChangeInvalidatesSessions).
const state = loadState();
const base = state.baseURL;

async function signInViaForm(page: import('@playwright/test').Page) {
  await page.goto(`${base}/login`);
  await page.getByLabel('Access token').fill(state.token);
  await Promise.all([
    page.waitForURL((u) => u.pathname !== '/login'),
    page.getByRole('button', { name: 'Sign In' }).click(),
  ]);
}

test.describe('token login (mobile, WebKit)', () => {
  test('1. a valid token signs in and opens a session', async ({ page }) => {
    await signInViaForm(page);
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();
    const session = (await page.context().cookies()).find((c) => c.name === 'sop_ctrl_session');
    expect(session, 'a session cookie must be set').toBeTruthy();
    expect(session!.httpOnly).toBe(true);
  });

  test('2. an invalid token is rejected with feedback', async ({ page }) => {
    await page.goto(`${base}/login`);
    await page.getByLabel('Access token').fill('definitely-not-the-token');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText('Invalid token')).toBeVisible();
    expect(new URL(page.url()).pathname).toBe('/login');
    expect((await page.context().cookies()).find((c) => c.name === 'sop_ctrl_session')).toBeFalsy();
  });

  test('3. authenticated navigation works across pages', async ({ page }) => {
    await signInViaForm(page);
    for (const path of ['/projects', '/approvals', '/discovery']) {
      await page.goto(`${base}${path}`);
      expect(new URL(page.url()).pathname).toBe(path);
    }
  });

  test('4. the session persists across reloads, with no token in storage', async ({ page }) => {
    await signInViaForm(page);
    await page.goto(`${base}/approvals`);
    await page.reload();
    expect(new URL(page.url()).pathname).toBe('/approvals');
    const storage = await page.evaluate(() => JSON.stringify(window.localStorage));
    expect(storage).not.toContain(state.token);
  });

  test('5. sign out ends the session', async ({ page }) => {
    await signInViaForm(page);
    await page.goto(`${base}/projects`);
    await Promise.all([
      page.waitForURL((u) => u.pathname === '/login'),
      page.getByRole('button', { name: 'Sign out' }).click(),
    ]);
    // The old session no longer authenticates.
    await page.goto(`${base}/approvals`);
    await page.waitForURL((u) => u.pathname === '/login');
    expect(new URL(page.url()).pathname).toBe('/login');
  });

  test('7. the login and logout POSTs are CSRF-protected', async ({ page, request }) => {
    const noCsrf = await request.post(`${base}/login`, {
      form: { token: state.token },
      maxRedirects: 0,
    });
    expect(noCsrf.status()).toBe(403);

    await signInViaForm(page);
    const logoutNoCsrf = await page.request.post(`${base}/logout`, { maxRedirects: 0 });
    expect(logoutNoCsrf.status()).toBe(403);
  });

  test('8. an unauthenticated approval action is blocked', async ({ request }) => {
    const res = await request.post(
      `${base}/projects/${state.projects.readonly.id}/tasks/${state.projects.readonly.task}/commands/approve`,
      { maxRedirects: 0 },
    );
    expect(res.status()).toBe(401);
  });

  test('9. the token never appears in the page, the URL, or storage', async ({ page }) => {
    await page.goto(`${base}/login`);
    expect(await page.content()).not.toContain(state.token);
    expect(page.url()).not.toContain(state.token);

    await signInViaForm(page);
    expect(page.url()).not.toContain(state.token);
    const storage = await page.evaluate(() => JSON.stringify(window.localStorage));
    expect(storage).not.toContain(state.token);
  });

  test('10. the login layout fits a phone viewport', async ({ page }) => {
    await page.goto(`${base}/login`);
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(overflow).toBeLessThanOrEqual(1);
    const input = page.getByLabel('Access token');
    await expect(input).toBeVisible();
    expect(await input.getAttribute('type')).toBe('password');
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  });
});
