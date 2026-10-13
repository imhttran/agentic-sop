import { expect, test, type Page } from "@playwright/test";
import { copiedText, stubClipboard } from "./support/clipboard";
import {
  NEEDS_HUMAN_REASON,
  sopApprove,
  sopApprovalState,
} from "./support/fixtures";
import { loadState } from "./support/state";

// The state global-setup wrote: the controller's base URL and token, and one
// disposable project per decision path.
const state = loadState();
const withToken = (p: string) => `${state.baseURL}${p}?token=${state.token}`;

// projectCard scopes to one project's section in the inbox, so the (unique) task id in
// each section is never ambiguous with another project's gate.
function projectCard(page: Page, id: string) {
  return page
    .locator("section.decisions")
    .filter({ has: page.getByRole("heading", { name: id, exact: true }) })
    .locator(".decisions-item");
}

test.describe("approvals inbox (mobile, WebKit)", () => {
  test.beforeEach(async ({ page }) => {
    // Make the clipboard deterministic for the whole file (Copy error / Copy details).
    await stubClipboard(page);
    await page.goto(withToken("/approvals"));
  });

  test("1. inbox renders on a phone viewport without horizontal overflow", async ({
    page,
  }) => {
    await expect(
      page.getByRole("heading", { name: "Pending approvals" }),
    ).toBeVisible();
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(
      overflow,
      "the page must not scroll horizontally on a phone",
    ).toBeLessThanOrEqual(1);
  });

  test("2. a pending gate shows enough context to decide", async ({ page }) => {
    const card = projectCard(page, state.projects.readonly.id);
    await expect(card).toHaveCount(1);
    await expect(
      card.getByRole("link", { name: state.projects.readonly.task }),
    ).toBeVisible();
    await expect(card).toContainText("NEEDS_HUMAN");
    await expect(card).toContainText(NEEDS_HUMAN_REASON);
    await expect(card).toContainText("Requested");
    await expect(card.getByRole("button", { name: "Approve" })).toBeVisible();
    await expect(card.getByRole("button", { name: "Decline" })).toBeVisible();
  });

  test("2b. each approval has exactly one shared Decision Note field", async ({
    page,
  }) => {
    const card = projectCard(page, state.projects.readonly.id);
    await expect(card).toHaveCount(1);
    await expect(card.locator("form")).toHaveCount(1);
    await expect(card.getByPlaceholder("by (actor, optional)")).toHaveCount(1);
    await expect(card.getByPlaceholder("decision note (optional)")).toHaveCount(
      1,
    );
    await expect(card.getByRole("button", { name: "Approve" })).toHaveCount(1);
    await expect(card.getByRole("button", { name: "Decline" })).toHaveCount(1);
  });

  test("8. mobile controls and the shared form stay usable", async ({
    page,
  }) => {
    const card = projectCard(page, state.projects.readonly.id);
    const by = card.getByPlaceholder("by (actor, optional)");
    const note = card.getByPlaceholder("decision note (optional)");
    await by.fill("phone tester");
    await note.fill("looks good");
    await expect(by).toHaveValue("phone tester");
    await expect(note).toHaveValue("looks good");

    const box = await by.boundingBox();
    const viewport = page.viewportSize();
    expect(box).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width + 1);
    await expect(card.getByRole("button", { name: "Approve" })).toBeEnabled();
    await expect(card.getByRole("button", { name: "Decline" })).toBeEnabled();
  });

  test("3. Approve submits the shared note and updates authoritative SOP state", async ({
    page,
  }) => {
    const fixture = state.projects.approve;
    const card = projectCard(page, fixture.id);
    await expect(card).toHaveCount(1);

    const form = card.locator("form.decisions-form");
    await form.getByPlaceholder("by (actor, optional)").fill("e2e");
    await form.getByPlaceholder("decision note (optional)").fill("approve e2e");
    await form.getByRole("button", { name: "Approve" }).click();

    await expect(page.locator("#approvals-cmd")).toContainText(
      "SOP recorded the approval decision.",
    );
    await expect(projectCard(page, fixture.id)).toHaveCount(0);

    const record = sopApprovalState(fixture.root, fixture.task);
    expect(record).toContain("Approval: APPROVED");
    expect(record).toContain("Decided by: e2e");
    expect(record).toContain("Note: approve e2e");
  });

  test("4. Decline submits the shared note and updates authoritative SOP state", async ({
    page,
  }) => {
    const fixture = state.projects.decline;
    const card = projectCard(page, fixture.id);
    await expect(card).toHaveCount(1);

    const form = card.locator("form.decisions-form");
    await form.getByPlaceholder("by (actor, optional)").fill("e2e");
    await form.getByPlaceholder("decision note (optional)").fill("decline e2e");
    await form.getByRole("button", { name: "Decline" }).click();

    await expect(page.locator("#approvals-cmd")).toContainText(
      "SOP recorded the decline/withhold decision.",
    );
    await expect(projectCard(page, fixture.id)).toHaveCount(0);

    const record = sopApprovalState(fixture.root, fixture.task);
    expect(record).toContain("Approval: DECLINED");
    expect(record).toContain("Decided by: e2e");
    expect(record).toContain("Note: decline e2e");
  });

  test("7. a stale decision is reported as a conflict", async ({ page }) => {
    const fixture = state.projects.stale;
    const card = projectCard(page, fixture.id);
    await expect(card).toHaveCount(1);

    // Another operator resolves the gate out-of-band; the page still shows it, so the
    // decision the UI submits is now stale. SOP must refuse it as a conflict, not a 500.
    sopApprove(fixture.root, fixture.task);
    expect(sopApprovalState(fixture.root, fixture.task)).toContain(
      "Approval: APPROVED",
    );

    await card.getByRole("button", { name: "Decline" }).click();
    await expect(page.locator("#approvals-cmd")).toContainText("conflict");

    // The command fragment offers Copy error, which copies the concise diagnostic.
    await page
      .locator("#approvals-cmd")
      .getByRole("button", { name: "Copy error" })
      .click();
    const copied = await copiedText(page);
    expect(copied).toContain("SOP command diagnostic");
    expect(copied).toContain("Result: conflict");
  });
});
