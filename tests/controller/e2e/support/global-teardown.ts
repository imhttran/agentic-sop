import fs from "node:fs";
import path from "node:path";
import { WORK_DIR } from "./state";

// global-teardown stops the controller the setup started and removes the disposable
// projects and work directory. It runs after the suite whether it passed or failed, so
// it never leaves a test server or fixture behind.
export default async function globalTeardown(): Promise<void> {
  const pidFile = path.join(WORK_DIR, "controller.pid");
  if (fs.existsSync(pidFile)) {
    const pid = Number(fs.readFileSync(pidFile, "utf8").trim());
    if (Number.isInteger(pid) && pid > 0) {
      try {
        process.kill(pid, "SIGTERM");
      } catch {
        // Already gone.
      }
      // Give it a moment to exit before the directory is removed.
      await new Promise((r) => setTimeout(r, 300));
    }
  }
  fs.rmSync(WORK_DIR, { recursive: true, force: true });
}
