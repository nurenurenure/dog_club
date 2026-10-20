import { useQueryClient } from '@tanstack/react-query';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react';
import { refreshSession } from '../api/refresh';
import { getSession, subscribeSession, type Credentials } from '../api/session';
import * as authApi from './authApi';

type Status = 'restoring' | 'anonymous' | 'authenticated';

interface AuthValue {
  status: Status;
  credentials: Credentials | null;
  login: (login: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  switchProfile: (profileID: string) => Promise<void>;
}

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const credentials = useSyncExternalStore(subscribeSession, getSession);
  const [restoring, setRestoring] = useState(true);

  // A reload loses the in-memory access token: restore it from the refresh cookie.
  useEffect(() => {
    let active = true;
    refreshSession()
      .catch(() => null)
      .finally(() => active && setRestoring(false));
    return () => {
      active = false;
    };
  }, []);

  // Cached data belongs to one account and profile; drop it when they change.
  const sessionKey = credentials ? `${credentials.user.id}:${credentials.profile.id}` : '';
  useEffect(() => {
    queryClient.clear();
  }, [sessionKey, queryClient]);

  const login = useCallback(async (name: string, password: string) => {
    await authApi.login(name, password);
  }, []);
  const logout = useCallback(() => authApi.logout(), []);
  const switchProfile = useCallback(async (profileID: string) => {
    await authApi.switchProfile(profileID);
  }, []);

  const value = useMemo<AuthValue>(
    () => ({
      status: credentials ? 'authenticated' : restoring ? 'restoring' : 'anonymous',
      credentials,
      login,
      logout,
      switchProfile,
    }),
    [credentials, restoring, login, logout, switchProfile],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used inside AuthProvider');
  return value;
}

/** The signed-in session; only for screens rendered behind RequireAuth. */
export function useSession(): Credentials {
  const { credentials } = useAuth();
  if (!credentials) throw new Error('useSession requires an authenticated session');
  return credentials;
}
