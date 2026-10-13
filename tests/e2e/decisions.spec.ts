import { expect, test } from "@playwright/test";
import { signIn, url } from "./support/auth";
import { loadState } from "./support/state";

// The project decisions panel (templates/partials/decisions.html + internal/web/decisions.go)
// was changed in 5ea10c2 to distinguish an unread listing from a reported-empty one and to
// label the empty case "none" instead of the absence label. These tests cover that panel
// through the real browser — the surface the audit flagged as a gap.
const state = loadState();

test.describe("project decisions panel (mobile, WebKit)", () => {
  test("shows a pending approval gate and its controls", async ({ page }) => {
    await signIn(page);
    await page.goto(url(`/projects/${state.projects.readonly.id}`));
    const panel = page.locator("#decisions");
    await expect(
      panel.getByRole("heading", { name: "Human decisions" }),
    ).toBeVisible();
    await expect(
      panel.getByRole("link", { name: state.projects.readonly.task }),
    ).toBeVisible();
    await expect(panel).toContainText("NEEDS_HUMAN");
    await expect(panel.getByRole("button", { name: "Approve" })).toHaveCount(1);
    await expect(panel.getByRole("button", { name: "Decline" })).toHaveCount(1);
  });

  test('a reported-empty listing reads "none", not the absence label', async ({
    page,
  }) => {
    await signIn(page);
    await page.goto(url(`/projects/${state.projects.empty.id}`));
    const gates = page
      .locator("#decisions p")
      .filter({ hasText: "Approval gates:" });
    await expect(gates).toContainText("none");
    await expect(gates).toContainText(
      "SOP reports no applicable approval gate for this project.",
    );
    // The reported-empty case must not fall back to the "not run" absence label.
    await expect(gates).not.toContainText("not run");
    // No gate means no decision control is offered.
    await expect(
      page.locator("#decisions").getByRole("button", { name: "Approve" }),
    ).toHaveCount(0);
    await expect(
      page.locator("#decisions").getByRole("button", { name: "Decline" }),
    ).toHaveCount(0);
  });
});
