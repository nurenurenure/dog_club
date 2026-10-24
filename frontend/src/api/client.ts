import createClient from 'openapi-fetch';
import { ApiError, toApiError } from './problem';
import { refreshSession } from './refresh';
import type { components, paths } from './schema';
import { getSession, setSession } from './session';

// Endpoints that never carry a bearer token or must not trigger a refresh retry.
const anonymous = ['/auth/login', '/auth/refresh', '/auth/activate', '/auth/forgot-password', '/auth/reset-password'];

function isAnonymous(url: string): boolean {
  const path = new URL(url, window.location.origin).pathname;
  return anonymous.some((suffix) => path.endsWith(suffix));
}

function withToken(request: Request): Request {
  const session = getSession();
  if (!session || isAnonymous(request.url)) return request;
  const headers = new Headers(request.headers);
  headers.set('Authorization', `Bearer ${session.access_token}`);
  return new Request(request, { headers });
}

async function send(request: Request): Promise<Response> {
  try {
    return await fetch(request);
  } catch {
    throw toApiError(0, null);
  }
}

let rechecking: Promise<void> | null = null;

/**
 * Re-reads the account and the current profile. A 403 may mean the administrator
 * disabled this profile or linked a member card meanwhile; the shell then explains it.
 */
export function reloadAccount(): Promise<void> {
  rechecking ??= readAccount()
    .catch(() => undefined)
    .finally(() => {
      rechecking = null;
    });
  return rechecking;
}

async function readAccount(): Promise<void> {
  const before = getSession();
  if (!before) return;
  const response = await send(withToken(new Request(new URL('/api/v1/auth/me', window.location.origin))));
  if (!response.ok) return;
  const me = (await response.json()) as components['schemas']['Principal'];
  const now = getSession();
  // A sign-out or another profile may have taken over meanwhile.
  if (!now || now !== before || me.user.id !== now.user.id || me.profile.id !== now.profile.id) return;
  if (me.profile.enabled === now.profile.enabled && me.user.club_member_id === now.user.club_member_id) return;
  setSession({ ...now, user: me.user, profile: me.profile });
}

/** Adds the bearer token; on 401 refreshes once and repeats the request, on 403 rechecks the account. */
export async function authFetch(request: Request): Promise<Response> {
  const retry = request.clone();
  const response = await send(withToken(request));
  if (isAnonymous(request.url) || !getSession()) {
    return response;
  }
  if (response.status === 403) void reloadAccount();
  if (response.status !== 401) {
    return response;
  }
  const fresh = await refreshSession();
  return fresh ? send(withToken(retry)) : response;
}

// An absolute same-origin base: Request objects need absolute URLs outside browsers.
export const api = createClient<paths>({
  baseUrl: window.location.origin,
  credentials: 'same-origin',
  fetch: authFetch,
});

type Result<T> = { data?: T; error?: unknown; response: Response };

/** Unwraps an openapi-fetch result: returns data or throws ApiError. */
export async function call<T>(pending: Promise<Result<T>>): Promise<T> {
  const { data, error, response } = await pending;
  if (!response.ok) throw toApiError(response.status, error);
  return data as T;
}

export { ApiError };
