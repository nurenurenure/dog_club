import type { Page } from '@playwright/test';
import { ROLES, type Role } from './support/accounts';
import { expect, open, settle, test, type Watch } from './support/fixtures';

/** Refusals a role meets by design while browsing. */
const EXPECTED: Partial<Record<Role, [number, RegExp][]>> = {
  // An owner sees a mate of their dog only by number: the partner's card belongs to someone else.
  owner: [[404, /^\/api\/v1\/dogs\/\d+$/]],
};

const MAX_PAGES = 70;
const MAX_DEPTH = 3;
const BLOCKED_TEXTS = ['Раздел недоступен', 'Запись не найдена или недоступна', 'Страница не найдена'];

/** /dogs/12?tab=owners → /dogs/:id, so one record of each kind is enough. */
function kind(href: string) {
  return href.split(/[?#]/)[0]!.replace(/\/\d+(?=\/|$)/g, '/:id');
}

/** Section links from the sidebar menu. */
async function sections(page: Page) {
  const nav = page.getByRole('navigation', { name: 'Разделы' }).first();
  return nav.locator('a[href]').evaluateAll((links) => links.map((link) => link.getAttribute('href')!));
}

/** In-app links of the main area that lead to records; downloads are buttons, not links. */
async function recordLinks(page: Page) {
  const hrefs = await page.locator('main a[href^="/"]').evaluateAll((links) => links.map((link) => link.getAttribute('href')!));
  return hrefs.filter((href) => /\/\d+(\/|\?|#|$)/.test(href) && !/\/(file|download|new)\b/.test(href));
}

async function checkScreen(page: Page, watch: Watch, where: string) {
  for (const text of BLOCKED_TEXTS) {
    if (await page.getByText(text, { exact: true }).count()) watch.problems.push(`${where}: «${text}»`);
  }
  for (const alert of await page.locator('main [role="alert"]').allInnerTexts()) watch.problems.push(`${where}: alert «${alert.trim()}»`);
}

/** Opens a page, every tab on it, and then one record of each kind it links to. */
async function crawl(page: Page, watch: Watch, start: string[]) {
  const seen = new Set<string>();
  let queue = start.map((href) => ({ href, depth: 0 }));
  let pages = 0;
  while (queue.length && pages < MAX_PAGES) {
    const { href, depth } = queue.shift()!;
    if (seen.has(kind(href))) continue;
    seen.add(kind(href));
    pages++;
    await open(page, href);
    await settle(page);
    await checkScreen(page, watch, href);
    const found = await recordLinks(page);
    const tabs = page.getByRole('tab');
    for (let i = 0; i < (await tabs.count()); i++) {
      const tab = tabs.nth(i);
      const name = (await tab.innerText()).trim();
      await tab.click();
      await settle(page);
      await checkScreen(page, watch, `${href} [${name}]`);
      found.push(...(await recordLinks(page)));
    }
    if (depth < MAX_DEPTH) queue = queue.concat(found.filter((link) => !seen.has(kind(link))).map((link) => ({ href: link, depth: depth + 1 })));
  }
  return pages;
}

for (const role of ROLES) {
  test(`${role}: every screen opens without refusals or errors`, async ({ sessions }) => {
    const { page, watch } = await sessions.as(role);
    watch.reset(EXPECTED[role]);
    await open(page, '/');
    await settle(page);
    const start = await sections(page);
    expect(start.length, 'menu sections').toBeGreaterThan(0);
    const pages = await crawl(page, watch, ['/', ...start, '/account']);
    test.info().annotations.push({ type: 'pages', description: String(pages) });
    expect(watch.problems).toEqual([]);
  });

  test(`${role}: sections fit a 375 px phone screen`, async ({ sessions }) => {
    const { page, watch } = await sessions.as(role);
    watch.reset(EXPECTED[role]);
    await page.setViewportSize({ width: 375, height: 812 });
    try {
      await open(page, '/');
      await settle(page);
      for (const href of ['/', ...(await sections(page))]) {
        await open(page, href);
        await settle(page);
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
        if (overflow > 1) watch.problems.push(`${href}: page is ${overflow}px wider than the screen`);
      }
    } finally {
      await page.setViewportSize({ width: 1280, height: 800 });
    }
    expect(watch.problems).toEqual([]);
  });
}
