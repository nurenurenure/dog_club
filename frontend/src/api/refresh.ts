import { toApiError } from './problem';
import { setSession, type Credentials } from './session';

const REFRESH_URL = '/api/v1/auth/refresh';
let inflight: Promise<Credentials | null> | null = null;

/**
 * Rotates the refresh cookie and stores the new access token.
 *
 * The backend treats two requests with the same refresh token as a replay and
 * revokes the session, so refreshes are serialized within the tab (one shared
 * promise) and across tabs (Web Locks). Resolves null when there is no session.
 */
export function refreshSession(): Promise<Credentials | null> {
  inflight ??= withLock(requestRefresh).finally(() => {
    inflight = null;
  });
  return inflight;
}

function withLock<T>(task: () => Promise<T>): Promise<T> {
  if (typeof navigator !== 'undefined' && navigator.locks) {
    return navigator.locks.request('dogclub-auth-refresh', task);
  }
  return task();
}

async function requestRefresh(): Promise<Credentials | null> {
  let response: Response;
  try {
    response = await fetch(REFRESH_URL, { method: 'POST', credentials: 'same-origin' });
  } catch {
    throw toApiError(0, null);
  }
  if (response.ok) {
    const credentials = (await response.json()) as Credentials;
    setSession(credentials);
    return credentials;
  }
  if (response.status === 401 || response.status === 403) {
    setSession(null);
    return null;
  }
  throw toApiError(response.status, await response.json().catch(() => null));
}
