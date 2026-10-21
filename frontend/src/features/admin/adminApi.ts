import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import { ApiError } from '../../api/problem';
import type { components } from '../../api/schema';
import type { Role } from '../../api/session';

type Schemas = components['schemas'];
export type User = Schemas['User'];
export type Profile = Schemas['Profile'];
export type Invitation = Schemas['Invitation'];
export type Instructor = Schemas['TrainingInstructor'];

export const adminKeys = {
  all: ['admin'] as const,
  users: ['admin', 'users'] as const,
  profiles: (userID: string) => ['admin', 'profiles', userID] as const,
  invitation: (userID: string) => ['admin', 'invitation', userID] as const,
  instructors: ['admin', 'instructors'] as const,
  instructorHistory: (profileID: string) => ['admin', 'instructor-history', profileID] as const,
  metrics: ['admin', 'metrics'] as const,
};

/** The whole account list: a club has dozens of accounts, pages of 100 are loaded in turn. */
export function useUsers() {
  return useQuery({
    queryKey: adminKeys.users,
    queryFn: async () => {
      const out: User[] = [];
      let after: string | undefined;
      for (let page = 0; page < 50; page += 1) {
        const result = await call(api.GET('/api/v1/users', { params: { query: { after, limit: 100 } } }));
        out.push(...result.users);
        if (result.users.length < 100) break;
        after = result.users[result.users.length - 1]!.id;
      }
      return out;
    },
  });
}

export function useUserProfiles(userID: string, enabled = true) {
  return useQuery({ queryKey: adminKeys.profiles(userID), enabled, queryFn: () => call(api.GET('/api/v1/users/{userID}/profiles', { params: { path: { userID } } })) });
}

/** The pending invitation, or null once the account is activated or the invitation cancelled. */
export function useUserInvitation(userID: string, enabled: boolean) {
  return useQuery({
    queryKey: adminKeys.invitation(userID),
    enabled,
    queryFn: async () => {
      try {
        return await call(api.GET('/api/v1/users/{userID}/invitation', { params: { path: { userID } } }));
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },
  });
}

/** Grants with the private basis: only the administrator reads it. */
export function useInstructorGrants() {
  return useQuery({
    queryKey: adminKeys.instructors,
    queryFn: async () => {
      const out: Instructor[] = [];
      let after: string | undefined;
      for (let page = 0; page < 20; page += 1) {
        const result = await call(api.GET('/api/v1/training-instructors', { params: { query: { after, limit: 50 } } }));
        out.push(...result.records);
        if (result.next_after === null) break;
        after = result.next_after;
      }
      return out;
    },
  });
}

export function useInstructorHistory(profileID: string) {
  return useInfiniteQuery({
    queryKey: adminKeys.instructorHistory(profileID),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/training-instructors/{profileID}/history', { params: { path: { profileID }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

export function useMetrics() {
  return useQuery({ queryKey: adminKeys.metrics, refetchInterval: 30_000, queryFn: () => call(api.GET('/api/v1/operations/metrics')) });
}

export function useAdminSaved() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: adminKeys.all });
}

const user = (userID: string) => ({ params: { path: { userID } } });
const invitation = (invitationID: string) => ({ params: { path: { invitationID } } });

export const adminApi = {
  invite: (body: Schemas['InviteInput']) => call(api.POST('/api/v1/invitations', { body })),
  resend: (id: string) => call(api.POST('/api/v1/invitations/{invitationID}/resend', invitation(id))),
  cancelInvitation: (id: string) => call(api.DELETE('/api/v1/invitations/{invitationID}', invitation(id))),
  setBlocked: (userID: string, blocked: boolean) => call(api.PATCH('/api/v1/users/{userID}', { ...user(userID), body: { blocked } })),
  bindMember: (userID: string, memberID: number) => call(api.PUT('/api/v1/users/{userID}/member', { ...user(userID), body: { member_id: memberID } })),
  addProfile: (userID: string, role: Role) => call(api.POST('/api/v1/users/{userID}/profiles', { ...user(userID), body: { role } })),
  setProfileEnabled: (userID: string, profileID: string, enabled: boolean) =>
    call(api.PATCH('/api/v1/users/{userID}/profiles/{profileID}', { params: { path: { userID, profileID } }, body: { enabled } })),
  setInstructor: (profileID: string, body: Schemas['TrainingInstructorInput']) =>
    call(api.PUT('/api/v1/training-instructors/{profileID}', { params: { path: { profileID } }, body })),
};
