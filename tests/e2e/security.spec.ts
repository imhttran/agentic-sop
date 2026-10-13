import { expect, test } from '@playwright/test';
import { loadState } from './support/state';

const state = loadState();
const withToken = (p: string) => `${state.baseURL}${p}?token=${state.token}`;

// The readonly project has a live pending gate, but every request below is rejected by
// auth or CSRF before the gate is touched, so it stays pending for the inbox tests.
const readonly = state.projects.readonly;
const approvePath = `/projects/${readonly.id}/tasks/${readonly.task}/commands/approve`;
const declinePath = `/projects/${readonly.id}/tasks/${readonly.task}/commands/decline`;

test.describe('authentication', () => {
  test('5a. rejects an unauthenticated request', async ({ request }) => {
    const res = await request.get(`${state.baseURL}/approvals`);
    expect(res.status()).toBe(401);
  });

  test('5b. rejects a wrong access token', async ({ request }) => {
    const res = await request.get(`${state.baseURL}/approvals`, {
      headers: { 'X-Access-Token': 'not-the-token' },
    });
    expect(res.status()).toBe(401);
  });

  test('5c. accepts the access token', async ({ request }) => {
    const res = await request.get(withToken('/approvals'));
    expect(res.status()).toBe(200);
  });
});

test.describe('CSRF protection', () => {
  test('6a. rejects a state-changing POST without a CSRF token', async ({ request }) => {
    const res = await request.post(`${state.baseURL}${approvePath}`, {
      headers: { 'X-Access-Token': state.token },
    });
    expect(res.status()).toBe(403);
  });

  test('6b. rejects a state-changing POST with a wrong CSRF token', async ({ request }) => {
    const res = await request.post(`${state.baseURL}${declinePath}`, {
      headers: { 'X-Access-Token': state.token },
      form: { csrf: 'not-the-csrf-token' },
    });
    expect(res.status()).toBe(403);
  });
});
