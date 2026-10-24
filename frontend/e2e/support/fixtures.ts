import { test as base, expect, type Browser, type BrowserContext, type Page, type Request, type Response } from '@playwright/test';
import { accountFor, password, type Role } from './accounts';
import { BASE_URL } from './env';

/** API refusals every role meets by design. */
const ALWAYS_ALLOWED: [number, RegExp][] = [
  [401, /\/api\/v1\/auth\/refresh$/], // the first visit has no session yet
  [404, /\/api\/v1\/dogs\/\d+\/photo$/], // a dog without a photo
];

/** Problems seen on one page: API refusals, script errors and console errors. */
export class Watch {
  problems: string[] = [];
  allowed: [number, RegExp][] = [];

  constructor(page: Page) {
    track(page);
    page.on('response', (response) => this.response(response));
    page.on('pageerror', (error) => this.problems.push(`page error: ${error.message}`));
    page.on('console', (message) => {
      // Failed requests are reported by status above, with their URL.
      if (message.type() === 'error' && !message.text().startsWith('Failed to load resource')) this.problems.push(`console: ${message.text()}`);
    });
  }

  private response(response: Response) {
    const url = new URL(response.url());
    if (!url.pathname.startsWith('/api/') || response.status() < 400) return;
    const allowed = [...ALWAYS_ALLOWED, ...this.allowed].some(([status, path]) => status === response.status() && path.test(url.pathname));
    if (!allowed) this.problems.push(`${response.status()} ${response.request().method()} ${url.pathname}${url.search}`);
  }

  /** Starts a fresh check; `allowed` adds refusals this check expects. */
  reset(allowed: [number, RegExp][] = []) {
    this.problems = [];
    this.allowed = allowed;
  }
}

export interface Session {
  context: BrowserContext;
  page: Page;
  watch: Watch;
  login: string;
}

/** Signs in through the login screen; waits out the per-minute limit when it hits it. */
export async function signIn(page: Page, login: string) {
  for (let attempt = 0; attempt < 3; attempt++) {
    await page.goto('/login');
    await page.getByLabel('Логин').fill(login);
    await page.getByLabel('Пароль', { exact: true }).fill(password());
    const [response] = await Promise.all([
      page.waitForResponse((r) => r.url().endsWith('/api/v1/auth/login')),
      page.getByRole('button', { name: 'Войти' }).click(),
    ]);
    if (response.status() === 429) {
      await page.waitForTimeout(61_000);
      continue;
    }
    expect(response.status(), `sign-in of ${login}`).toBe(200);
    await page.waitForURL((url) => url.pathname !== '/login');
    return;
  }
  throw new Error(`${login}: still rate limited`);
}

/** One signed-in browser context per role, kept for the whole worker. */
class Sessions {
  private open = new Map<Role, Session>();

  constructor(private browser: Browser) {}

  async as(role: Role): Promise<Session> {
    const known = this.open.get(role);
    if (known) return known;
    const context = await this.browser.newContext({
      baseURL: BASE_URL,
      viewport: { width: 1280, height: 800 },
      locale: 'ru-RU',
      timezoneId: 'Asia/Novosibirsk',
    });
    const page = await context.newPage();
    const watch = new Watch(page);
    const { login } = accountFor(role);
    await signIn(page, login);
    const session = { context, page, watch, login };
    this.open.set(role, session);
    return session;
  }

  async close() {
    await Promise.all([...this.open.values()].map((session) => session.context.close()));
  }
}

export const test = base.extend<object, { sessions: Sessions }>({
  sessions: [
    async ({ browser }, use) => {
      const sessions = new Sessions(browser);
      await use(sessions);
      await sessions.close();
    },
    { scope: 'worker' },
  ],
});

export { expect };

/** Requests still running on each page; screens opened without a reload give no load events to wait for. */
const running = new WeakMap<Page, Set<Request>>();

function track(page: Page) {
  const pending = new Set<Request>();
  running.set(page, pending);
  page.on('request', (request) => pending.add(request));
  // A response is enough: Chromium skips the finished event for some bodies nobody reads.
  page.on('response', (response) => pending.delete(response.request()));
  page.on('requestfinished', (request) => pending.delete(request));
  page.on('requestfailed', (request) => pending.delete(request));
}

/** Waits until the page has had no requests running for `still` ms. */
async function quiet(page: Page, still = 300, limit = 15_000) {
  const pending = running.get(page);
  if (!pending) return;
  const end = Date.now() + limit;
  let calm = Date.now();
  while (Date.now() < end) {
    if (pending.size) calm = Date.now();
    else if (Date.now() - calm >= still) return;
    await page.waitForTimeout(50);
  }
  throw new Error(`requests still running: ${[...pending].map((request) => request.url()).join(', ')}`);
}

/**
 * Opens an app address the way a link does: the router switches screens and the page is not reloaded.
 * Every reload refreshes the session, and the server allows 60 refreshes a minute from one address.
 */
export async function open(page: Page, href: string) {
  if (new URL(page.url()).origin !== new URL(BASE_URL).origin) {
    await page.goto(href);
    return;
  }
  await page.evaluate((path) => {
    // React Router keeps its place in history.state; the new entry continues that count.
    const state = window.history.state as { idx?: number } | null;
    window.history.pushState({ usr: null, key: Math.random().toString(36).slice(2, 10), idx: (state?.idx ?? 0) + 1 }, '', path);
    window.dispatchEvent(new PopStateEvent('popstate', { state: window.history.state }));
  }, href);
}

/** Waits until requests stop and no loading indicator is left. */
export async function settle(page: Page) {
  // Load events come once per document; screens opened by the router are tracked by quiet().
  await page.waitForLoadState('load');
  await quiet(page);
  await expect(page.getByRole('status').filter({ hasText: /^Загрузка/ })).toHaveCount(0);
  // A loaded screen may ask for more, such as a dog's photo or name.
  await quiet(page);
}
