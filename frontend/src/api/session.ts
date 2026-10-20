import type { components } from './schema';

export type Credentials = components['schemas']['Credentials'];
export type Profile = components['schemas']['Profile'];
export type Role = Profile['role'];
export type User = components['schemas']['User'];

// The access token lives only in memory; the refresh token is an HttpOnly cookie.
let current: Credentials | null = null;
const listeners = new Set<() => void>();

export function getSession(): Credentials | null {
  return current;
}

export function setSession(next: Credentials | null): void {
  current = next;
  listeners.forEach((listener) => listener());
}

export function subscribeSession(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
