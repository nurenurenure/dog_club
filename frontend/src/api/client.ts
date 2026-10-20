import createClient from 'openapi-fetch';
import { ApiError, toApiError } from './problem';
import { refreshSession } from './refresh';
import type { paths } from './schema';
import { getSession } from './session';

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

/** Adds the bearer token; on 401 refreshes once and repeats the request. */
export async function authFetch(request: Request): Promise<Response> {
  const retry = request.clone();
  const response = await send(withToken(request));
  if (response.status !== 401 || isAnonymous(request.url) || !getSession()) {
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
