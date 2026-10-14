import { defineConfig, devices } from '@playwright/test';

// End-to-end tests for the SOP Controller dashboard. These are development-only:
// they exercise the real controller binary over HTTP against disposable SOP projects
// and the installed `sop` CLI, so they never touch the Go build or the Go tests.
//
// The controller is NOT started by Playwright's `webServer` fixture: its port is
// allocated dynamically and it must observe the disposable SOP projects that
// `global-setup` creates first. `global-setup` builds and starts it, and
// `global-teardown` stops it and removes the fixtures; the tests read the resulting
// base URL, token, and project ids from the state file it writes.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['list']],
  outputDir: 'test-results',
  globalSetup: './e2e/support/global-setup.ts',
  globalTeardown: './e2e/support/global-teardown.ts',
  use: {
    // Capture evidence on failure; a passing run stays quiet.
    screenshot: 'only-on-failure',
    trace: 'on-first-retry',
  },
  // WebKit with an iPhone-sized viewport, per the task's emulation requirement.
  projects: [
    {
      name: 'webkit-iphone',
      use: { ...devices['iPhone 13'] },
    },
  ],
});
