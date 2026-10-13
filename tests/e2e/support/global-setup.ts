import fs from 'node:fs';
import path from 'node:path';
import type { E2EState } from './state';
import { STATE_FILE, WORK_DIR } from './state';
import {
  buildController,
  createPendingApproval,
  freePort,
  randomToken,
  startController,
  waitForHealth,
} from './fixtures';

// global-setup builds the controller, creates disposable SOP projects each parked at a
// genuine pending approval, starts the controller on a dynamically allocated loopback
// port with an isolated token, and writes the state the tests read.
export default async function globalSetup(): Promise<void> {
  fs.rmSync(WORK_DIR, { recursive: true, force: true });
  fs.mkdirSync(WORK_DIR, { recursive: true });

  const bin = buildController();
  const token = randomToken();
  const port = await freePort();

  // Distinct task ids per fixture keep the inbox cards unambiguous.
  const projects = {
    approve: createPendingApproval('approve', 'A1'),
    decline: createPendingApproval('decline', 'D1'),
    stale: createPendingApproval('stale', 'S1'),
    readonly: createPendingApproval('readonly', 'R1'),
  };

  const roots = Object.values(projects).map((p) => p.root);
  const child = startController(bin, roots, port, token);

  const baseURL = `http://127.0.0.1:${port}`;
  await waitForHealth(baseURL);

  fs.writeFileSync(path.join(WORK_DIR, 'controller.pid'), String(child.pid ?? ''));
  const state: E2EState = { baseURL, token, sopBin: process.env.SOP_BIN ?? 'sop', projects };
  fs.writeFileSync(STATE_FILE, JSON.stringify(state, null, 2));
}
