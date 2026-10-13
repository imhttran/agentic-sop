import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import fs from "node:fs";
import net from "node:net";
import path from "node:path";
import type { ApprovalFixture } from "./state";
import { REPO_ROOT, WORK_DIR } from "./state";

// The exact SOP-reported reason the fixture agent raises, so the tests can assert the
// inbox shows real decision context.
export const NEEDS_HUMAN_REASON =
  "the requested change is destructive and cannot be undone; it needs an explicit human decision before proceeding";

const FAKE_AGENT = path.join(
  REPO_ROOT,
  "tests",
  "e2e",
  "fixtures",
  "fake-agent.sh",
);
const PRD_TEMPLATE = path.join(REPO_ROOT, "tests", "e2e", "fixtures", "prd.md");

function run(
  cmd: string,
  args: string[],
  opts: { cwd?: string; env?: NodeJS.ProcessEnv } = {},
): string {
  return execFileSync(cmd, args, {
    cwd: opts.cwd,
    env: opts.env ?? process.env,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function rand(): string {
  return Math.random().toString(36).slice(2, 8);
}

// planJSON is the single-stage machine plan the fixture agent returns for a PLAN
// request, so `sop run` creates exactly one task and parks it at a human gate.
function planJSON(project: string, task: string): string {
  return JSON.stringify({
    project,
    summary:
      "Disposable fixture that parks one task at a genuine human approval gate.",
    stages: [
      {
        id: task,
        title: "Approval gate",
        objective: "Perform a change SOP must escalate to a human decision.",
        dependencies: [],
        deliverables: ["docs/result.md"],
        acceptance_criteria: ["The task reaches a human decision boundary."],
        execution_mode: "implement",
      },
    ],
  });
}

// createPendingApproval builds a disposable SOP project and drives it with the real
// `sop` CLI to a genuine pending approval gate — no SOP state is fabricated and no
// model is called (the agent is an SOP-supported command double).
export function createPendingApproval(
  kind: string,
  task: string,
): ApprovalFixture {
  const id = `e2e-${kind}-${rand()}`;
  const root = path.join(WORK_DIR, "projects", id);
  fs.mkdirSync(path.join(root, "docs"), { recursive: true });

  run("sop", ["init"], { cwd: root });

  // Patch the project identity and switch the agent to the command harness, so the
  // fixture agent is used without a provider.
  const cfgPath = path.join(root, ".agent-sdlc", "config.yaml");
  let cfg = fs.readFileSync(cfgPath, "utf8");
  cfg = cfg.replace(/^(\s*)name:\s*.*$/m, `$1name: "${id}"`);
  cfg = cfg.replace(/^(\s*)harness:\s*tool\s*$/m, "$1harness: command");
  cfg = cfg.replace(/^(\s*)provider:\s*ollama\s*$/m, "$1provider: command");
  fs.writeFileSync(cfgPath, cfg);

  fs.copyFileSync(PRD_TEMPLATE, path.join(root, "docs", "PRD.md"));

  // Drive SOP to the human boundary. `sop run` exits non-zero at a gate; that is the
  // expected outcome, so the gate listing below is the real assertion.
  try {
    run("sop", ["run"], {
      cwd: root,
      env: {
        ...process.env,
        SOP_AGENT_COMMAND: `sh "${FAKE_AGENT}"`,
        E2E_PLAN_JSON: planJSON(id, task),
      },
    });
  } catch {
    // The gate is verified next; a non-zero exit here is not itself a failure.
  }

  const listing = JSON.parse(
    run("sop", ["approvals", "--json"], { cwd: root }),
  ) as {
    approvals: Array<{ task_id: string; status: string }>;
  };
  const gate = listing.approvals.find(
    (a) => a.task_id === task && a.status === "PENDING",
  );
  if (!gate) {
    throw new Error(
      `fixture ${id}: expected a pending approval for ${task}, got ` +
        JSON.stringify(listing),
    );
  }
  return { id, root, task };
}

// buildController compiles the controller binary into the work directory. The Go
// toolchain is the only build dependency; Node is never required to build the app.
export function buildController(): string {
  const binDir = path.join(WORK_DIR, "bin");
  fs.mkdirSync(binDir, { recursive: true });
  const bin = path.join(binDir, "sop-controller");
  run("go", ["build", "-o", bin, "./cmd/sop-controller"], { cwd: REPO_ROOT });
  return bin;
}

// freePort asks the OS for an unused loopback port, so concurrent tooling never
// collides with the test controller.
export function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.once("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const address = srv.address();
      if (address && typeof address === "object") {
        const { port } = address;
        srv.close(() => resolve(port));
      } else {
        srv.close(() => reject(new Error("could not allocate a port")));
      }
    });
  });
}

// startController runs the controller in network mode bound to loopback, so the access
// token is enforced. It runs from the work directory, so the repository's own `.env`
// (developer credentials) is never loaded.
export function startController(
  bin: string,
  roots: string[],
  port: number,
  token: string,
): ChildProcess {
  const child = spawn(bin, [], {
    cwd: WORK_DIR,
    env: {
      ...process.env,
      SOP_CONTROLLER_PROJECTS: roots.join(","),
      SOP_CONTROLLER_ALLOW_NETWORK: "true",
      SOP_CONTROLLER_TOKEN: token,
      SOP_CONTROLLER_ADDR: `127.0.0.1:${port}`,
      SOP_BIN: process.env.SOP_BIN ?? "sop",
    },
    stdio: ["ignore", "inherit", "inherit"],
  });
  return child;
}

export async function waitForHealth(
  baseURL: string,
  timeoutMs = 20000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown = null;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseURL}/healthz`);
      if (res.ok) return;
      lastErr = new Error(`healthz ${res.status}`);
    } catch (err) {
      lastErr = err;
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(
    `controller did not become healthy at ${baseURL}: ${String(lastErr)}`,
  );
}

// sopApprovalState returns `sop approval <task>` output — SOP's authoritative record for
// one task's gate.
export function sopApprovalState(root: string, task: string): string {
  return run("sop", ["approval", task], { cwd: root });
}

// sopApprove resolves a gate out-of-band, standing in for another operator so a stale
// decision can be exercised.
export function sopApprove(root: string, task: string): void {
  try {
    run("sop", ["approve", task], { cwd: root });
  } catch {
    // A re-decision may already be resolved; the caller asserts the resulting state.
  }
}

export function randomToken(): string {
  return `e2e-${rand()}${rand()}`;
}
