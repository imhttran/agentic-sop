import type { Page } from "@playwright/test";

// The copy handler reads the text from a `data-copy` attribute and hands it to
// navigator.clipboard.writeText. These helpers replace that boundary with a
// deterministic in-page recorder so the tests assert the exact copied text without
// depending on real clipboard permissions or the platform clipboard. They must run
// before the page navigates.
export async function stubClipboard(page: Page): Promise<void> {
  await page.addInitScript(() => {
    (window as unknown as { __copied: string | null }).__copied = null;
    const impl = {
      writeText: (text: string) => {
        (window as unknown as { __copied: string }).__copied = text;
        return Promise.resolve();
      },
    };
    try {
      Object.defineProperty(navigator, "clipboard", { configurable: true, get: () => impl });
    } catch {
      (navigator as unknown as { clipboard: unknown }).clipboard = impl;
    }
  });
}

// stubClipboardFailure makes both the async Clipboard API and the execCommand fallback
// fail, so the copy-feedback path (the "Copy failed" message) can be asserted.
export async function stubClipboardFailure(page: Page): Promise<void> {
  await page.addInitScript(() => {
    (window as unknown as { __copied: string | null }).__copied = null;
    const impl = { writeText: () => Promise.reject(new Error("clipboard unavailable")) };
    try {
      Object.defineProperty(navigator, "clipboard", { configurable: true, get: () => impl });
    } catch {
      (navigator as unknown as { clipboard: unknown }).clipboard = impl;
    }
    document.execCommand = () => false;
  });
}

// copiedText is what the page most recently passed to the clipboard.
export async function copiedText(page: Page): Promise<string> {
  return (await page.evaluate(
    () => (window as unknown as { __copied: string | null }).__copied ?? "",
  )) as string;
}
