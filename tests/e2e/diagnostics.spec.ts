import { expect, test, type Page } from "@playwright/test";
import { signIn, url } from "./support/auth";
import {
  copiedText,
  stubClipboard,
  stubClipboardFailure,
} from "./support/clipboard";
import { loadState } from "./support/state";

// The Copy error / Copy details controls are server-authored: the text they copy is
// assembled from SOP-reported, non-sensitive fields and carried in a data attribute.
// These tests stub the browser clipboard boundary and assert the exact text, the
// feedback, and the secret-exclusion guarantee.
const state = loadState();

function approvalCard(page: Page, id: string) {
  return page
    .locator("section.decisions")
    .filter({ has: page.getByRole("heading", { name: id, exact: true }) })
    .locator(".decisions-item");
}

test.describe("diagnostic copy actions (mobile, WebKit)", () => {
  test("Copy details writes a structured, secret-free approval summary", async ({
    page,
  }) => {
    await stubClipboard(page);
    await signIn(page);
    await page.goto(url("/approvals"));

    const card = approvalCard(page, state.projects.readonly.id);
    await card.getByRole("button", { name: "Copy approval details" }).click();
    await expect(card.getByText("Copied")).toBeVisible();

    const copied = await copiedText(page);
    expect(copied).toContain(`Project: ${state.projects.readonly.id}`);
    expect(copied).toContain(`Task: ${state.projects.readonly.task}`);
    expect(copied).toContain("Gate: NEEDS_HUMAN");
    expect(copied).toContain("Reason: the requested change is destructive");
    expect(copied).toContain("\n"); // multiline formatting is preserved

    // No secrets: the access token and the CSRF token never reach the clipboard.
    expect(copied).not.toContain(state.token);
    const csrf = (await page.context().cookies()).find(
      (c) => c.name === "sop_ctrl_csrf",
    )?.value;
    if (csrf) expect(copied).not.toContain(csrf);
  });

  test("Copy error copies the error-page diagnostic", async ({ page }) => {
    await stubClipboard(page);
    await signIn(page);
    await page.goto(url("/projects/does-not-exist"));

    await expect(page.getByRole("heading", { name: "404" })).toBeVisible();
    await page.getByRole("button", { name: "Copy error" }).click();
    await expect(page.getByText("Copied")).toBeVisible();

    const copied = await copiedText(page);
    expect(copied).toContain("SOP Controller error");
    expect(copied).toContain("Status: 404");
    expect(copied).toContain("Message: Project not found");
    expect(copied).not.toContain(state.token);
  });

  test("copy reports failure when the clipboard is unavailable", async ({
    page,
  }) => {
    await stubClipboardFailure(page);
    await signIn(page);
    await page.goto(url("/approvals"));

    const card = approvalCard(page, state.projects.readonly.id);
    await card.getByRole("button", { name: "Copy approval details" }).click();
    await expect(card.getByText("Copy failed")).toBeVisible();
  });
});
