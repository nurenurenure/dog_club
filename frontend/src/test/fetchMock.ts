import { vi } from 'vitest';

type Handler = (request: Request) => Response | Promise<Response>;

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json' } });
}

export function problem(status: number, code: string, detail: string): Response {
  return json({ type: 'about:blank', title: 'Error', status, code, detail, request_id: 'test-request' }, status);
}

/** Routes fetch by "METHOD /path"; records every request for assertions. */
export function mockFetch(routes: Record<string, Handler>) {
  const calls: Request[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : new Request(new URL(String(input), 'http://localhost'), init);
    calls.push(request.clone());
    const key = `${request.method} ${new URL(request.url).pathname}`;
    const handler = routes[key];
    if (!handler) throw new Error(`Unexpected request ${key}`);
    return handler(request);
  });
  vi.stubGlobal('fetch', fetchMock);
  return { calls, fetchMock };
}

export const staffCredentials = {
  access_token: 'access-staff',
  token_type: 'Bearer',
  expires_in: 600,
  user: { id: 'u1', login: 'ivanova.m', email: 'ivanova@dogclub.test', status: 'active', created_at: '2026-01-01T00:00:00Z' },
  profile: { id: 'p-staff', role: 'staff', enabled: true },
} as const;
