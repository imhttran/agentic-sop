import fs from "node:fs";
import path from "node:path";

// The repository root, resolved from this module's location (tests/e2e/support).
export const REPO_ROOT = path.resolve(__dirname, "..", "..", "..");

// Everything the E2E run creates lives under this ignored directory so it can never
// pollute the repository or be confused with real projects.
export const WORK_DIR = path.join(REPO_ROOT, ".e2e");

export const STATE_FILE = path.join(WORK_DIR, "state.json");

// One disposable SOP project. `task` is the task parked at a human gate, or "" for a
// project with no gate (whose listing is a reported-empty set).
export interface ApprovalFixture {
  // The controller's project id (the fixture's declared project name).
  id: string;
  // The fixture's project root on disk.
  root: string;
  // The task id parked at the gate, or "" when the project has no gate.
  task: string;
}

export interface E2EState {
  baseURL: string;
  token: string;
  sopBin: string;
  // Path to the controller binary global-setup built, so a test can restart its own
  // dedicated instance (restart/rotation coverage).
  bin: string;
  projects: {
    approve: ApprovalFixture;
    decline: ApprovalFixture;
    stale: ApprovalFixture;
    readonly: ApprovalFixture;
    // empty declares no approval gate, so the decisions panel reports "none".
    empty: ApprovalFixture;
  };
}

// loadState reads the state global-setup wrote; it throws (failing loudly) when the
// setup did not run, rather than letting a test silently assert against nothing.
export function loadState(): E2EState {
  if (!fs.existsSync(STATE_FILE)) {
    throw new Error(
      `E2E state not found at ${STATE_FILE}; global-setup did not complete. ` +
        "Run the suite through `npm run test:e2e` so the setup runs.",
    );
  }
  return JSON.parse(fs.readFileSync(STATE_FILE, "utf8")) as E2EState;
}
