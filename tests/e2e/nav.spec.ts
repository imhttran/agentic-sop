import { expect, test } from "@playwright/test";
import { loadState } from "./support/state";
import { signIn } from "./support/auth";

// Responsive header regression. On phone widths the top bar must stay readable and
// touch-friendly: the brand and the Sign out button must not wrap or be squeezed, and the
// header must not push the page into horizontal overflow. The desktop layout is guarded by
// the fact that these rules live inside a max-width media query.
const state = loadState();
const widths = [320, 375, 430];

test.describe("responsive header (mobile, WebKit)", () => {
  for (const width of widths) {
    test(`top bar stays wrap-free and overflow-free at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 780 });
      await signIn(page);
      await page.goto(`${state.baseURL}/approvals`);

      // The page never scrolls sideways at any phone width.
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `horizontal overflow at ${width}px`).toBeLessThanOrEqual(
        1,
      );

      const brand = page.locator(".brand");
      const nav = page.locator(".nav");
      const signout = page.getByRole("button", { name: "Sign out" });
      await expect(brand).toBeVisible();
      await expect(signout).toBeVisible();

      // The header keeps one predictable height (it does not balloon as items wrap).
      const header = await page.locator("header.topbar").boundingBox();
      expect(header, "header must render").not.toBeNull();
      expect(
        header!.height,
        `header too tall at ${width}px`,
      ).toBeLessThanOrEqual(96);

      // Identity line: the brand is a single line and is not clipped by a shrink.
      const brandBox = await brand.boundingBox();
      expect(brandBox!.height, `brand wrapped at ${width}px`).toBeLessThan(30);
      const brandClipped = await brand.evaluate(
        (el) => el.scrollWidth > el.clientWidth + 1,
      );
      expect(brandClipped, `brand clipped at ${width}px`).toBe(false);

      // Sign out is a comfortable, single-line tap target, not a narrow squeezed button.
      const signoutBox = await signout.boundingBox();
      expect(
        signoutBox!.width,
        `sign out too narrow at ${width}px`,
      ).toBeGreaterThanOrEqual(64);
      expect(signoutBox!.height, `sign out wrapped at ${width}px`).toBeLessThan(
        40,
      );

      // Every nav link is present and on one line.
      for (const label of ["Projects", "Approvals", "Discovery"]) {
        const link = nav.getByRole("link", { name: label });
        await expect(link).toBeVisible();
        expect(
          (await link.boundingBox())!.height,
          `${label} wrapped at ${width}px`,
        ).toBeLessThan(40);
      }
    });
  }

  test("desktop keeps the single-row header (the phone rule does not leak)", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 800 });
    await signIn(page);
    await page.goto(`${state.baseURL}/approvals`);

    const brand = await page.locator(".brand").boundingBox();
    const nav = await page.locator(".nav").boundingBox();
    expect(brand && nav, "brand and nav must render").toBeTruthy();
    // Brand and nav share one row on desktop (the phone stacking must not apply >640px).
    expect(
      Math.abs(brand!.y - nav!.y),
      "nav wrapped below the brand on desktop",
    ).toBeLessThan(12);
    expect(
      (await page.locator("header.topbar").boundingBox())!.height,
    ).toBeLessThan(60);
  });
});
