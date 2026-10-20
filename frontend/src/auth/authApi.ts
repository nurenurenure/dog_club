import { api, call } from '../api/client';
import type { components } from '../api/schema';
import { setSession, type Credentials } from '../api/session';

export type Profiles = components['schemas']['Profiles'];

export async function login(login: string, password: string): Promise<Credentials> {
  const credentials = await call(api.POST('/api/v1/auth/login', { body: { login, password } }));
  setSession(credentials);
  return credentials;
}

export function activate(token: string, password: string): Promise<void> {
  return call(api.POST('/api/v1/auth/activate', { body: { token, password } }));
}

export function forgotPassword(login: string): Promise<{ message: string }> {
  return call(api.POST('/api/v1/auth/forgot-password', { body: { login } }));
}

export function resetPassword(token: string, password: string): Promise<void> {
  return call(api.POST('/api/v1/auth/reset-password', { body: { token, password } }));
}

export function listProfiles(): Promise<Profiles> {
  return call(api.GET('/api/v1/auth/profiles'));
}

export async function switchProfile(profileID: string): Promise<Credentials> {
  const credentials = await call(api.POST('/api/v1/auth/switch-profile', { body: { profile_id: profileID } }));
  setSession(credentials);
  return credentials;
}

/** Revokes this session; the local session is cleared even if the server is unreachable. */
export async function logout(): Promise<void> {
  try {
    await call(api.POST('/api/v1/auth/logout'));
  } finally {
    setSession(null);
  }
}

/** The server revokes every session after a password change; the caller then ends this one. */
export function changePassword(currentPassword: string, password: string): Promise<void> {
  return call(api.POST('/api/v1/auth/change-password', { body: { current_password: currentPassword, password } }));
}

export function endLocalSession(): void {
  setSession(null);
}
