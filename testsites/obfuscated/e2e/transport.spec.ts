import { expect, test } from "@playwright/test";

test.beforeEach(({ page }) => {
  page.on("pageerror", (err) => { throw err; });
});

test("pages travel as Malbolge and are decoded by the service worker", async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => navigator.serviceWorker.ready);
  await page.reload(); // now controlled by the worker
  await expect(page.locator("#state")).toHaveText(/decoded by the service worker/);

  const about = await page.evaluate(async () => {
    const r = await fetch("/about.html");
    return { decoded: r.headers.get("X-Malbolge-Decoded"), bytes: r.headers.get("X-Malbolge-Program-Bytes"),
      type: r.headers.get("Content-Type"), text: await r.text() };
  });
  expect(about.decoded).toBe("service-worker");
  expect(Number(about.bytes)).toBeGreaterThan(about.text.length * 3);
  expect(about.type).toBe("text/html; charset=utf-8");
  expect(about.text).toContain("Ünïcødé ✓ survives the trip");

  // The raw wire format really is Malbolge (Playwright's request client bypasses the worker).
  const raw = await page.request.get("/about.html", { headers: { "X-Malbolge-Accept": "program" } });
  expect(raw.headers()["x-malbolge-encoding"]).toBe("program");
  const wire = await raw.text();
  expect(wire).not.toContain("About the transport");
  expect(wire).toMatch(/^[!-~\n]+$/); // only Malbolge program text

  // Navigations and binary subresources are decoded too.
  await page.goto("/about.html");
  await expect(page.locator("h1")).toHaveText("About the transport");
  await page.goto("/");
  const width = await page.locator("img").evaluate((img: HTMLImageElement) =>
    img.complete ? img.naturalWidth : new Promise((ok) => (img.onload = () => ok(img.naturalWidth))));
  expect(width).toBe(48);
});

test("playground runs and compiles Malbolge in WebAssembly", async ({ page }) => {
  await page.goto("/_melhttp/playground.html");
  await expect(page.locator("#status")).toHaveText(/VM ready/);
  await page.locator("#run").click();
  await expect(page.locator("#output")).toHaveText("Hello, world.");
  await expect(page.locator("#runStats")).toContainText("halted");

  await page.locator("#text").fill("Malbolge in your browser ✓");
  await page.locator("#compile").click();
  await expect(page.locator("#compileStats")).toContainText("program(s)", { timeout: 60_000 });
  await page.locator("#useIt").click();
  await expect(page.locator("#output")).toHaveText("Malbolge in your browser ✓");
});
