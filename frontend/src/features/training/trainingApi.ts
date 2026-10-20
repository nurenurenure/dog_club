import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import type { components } from '../../api/schema';
import { useSession } from '../../auth/AuthProvider';

type Schemas = components['schemas'];
export type TrainingSession = Schemas['TrainingSession'];
export type TrainingInput = Schemas['TrainingInput'];
export type TrainingUpdate = Schemas['TrainingUpdate'];
export type MedicalCheck = Schemas['TrainingMedicalCheck'];

export interface TrainingFilter {
  dog_id?: number;
  type_id?: number;
  instructor_profile_id?: string;
  from?: string;
  to?: string;
}

export const trainingKeys = {
  all: ['training'] as const,
  list: (filter: TrainingFilter) => ['training', 'list', filter] as const,
  session: (dogID: number, id: number) => ['training', 'session', dogID, id] as const,
  history: (dogID: number, id: number) => ['training', 'history', dogID, id] as const,
  types: ['training-types'] as const,
  instructors: ['training-instructors'] as const,
};

export function useTrainingTypes() {
  return useQuery({ queryKey: trainingKeys.types, staleTime: Infinity, queryFn: () => call(api.GET('/api/v1/training-types')) });
}

export function useTrainingSessions(filter: TrainingFilter) {
  return useInfiniteQuery({
    queryKey: trainingKeys.list(filter),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/training-sessions', { params: { query: { ...compact(filter), after: pageParam || undefined, limit: 50 } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

const path = (dogID: number, sessionID: number) => ({ params: { path: { dogID, sessionID } } });

export function useTrainingSession(dogID: number, id: number) {
  return useQuery({ queryKey: trainingKeys.session(dogID, id), queryFn: () => call(api.GET('/api/v1/dogs/{dogID}/training-sessions/{sessionID}', path(dogID, id))) });
}

export function useTrainingHistory(dogID: number, id: number) {
  return useInfiniteQuery({
    queryKey: trainingKeys.history(dogID, id),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      call(api.GET('/api/v1/dogs/{dogID}/training-sessions/{sessionID}/history', { params: { path: { dogID, sessionID: id }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

/** Instructor grants are short; staff and leader read them without the private basis. */
export function useInstructors(enabled: boolean) {
  return useQuery({
    queryKey: trainingKeys.instructors,
    enabled,
    staleTime: 60_000,
    queryFn: async () => {
      const out: Schemas['TrainingInstructor'][] = [];
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

/**
 * The grant of the current staff profile: training types it may record now.
 * Being staff is not enough; the administrator grants instructors separately.
 */
export function useOwnGrant() {
  const { profile } = useSession();
  const staff = profile.role === 'staff';
  const instructors = useInstructors(staff);
  const grant = instructors.data?.find((item) => item.profile_id === profile.id && item.enabled && item.available);
  return { pending: staff && instructors.isPending, typeIDs: grant?.type_ids ?? [] };
}

export function useTrainingSaved() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: trainingKeys.all });
}

export const trainingApi = {
  create: (dogID: number, body: TrainingInput) => call(api.POST('/api/v1/dogs/{dogID}/training-sessions', { params: { path: { dogID } }, body })),
  update: (dogID: number, sessionID: number, body: TrainingUpdate) => call(api.PUT('/api/v1/dogs/{dogID}/training-sessions/{sessionID}', { ...path(dogID, sessionID), body })),
  void: (dogID: number, sessionID: number, version: number, reason: string) =>
    call(api.POST('/api/v1/dogs/{dogID}/training-sessions/{sessionID}/void', { ...path(dogID, sessionID), body: { version, reason } })),
};

function compact(filter: TrainingFilter) {
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(filter)) {
    if (value !== undefined && value !== '' && value !== 0) out[key] = value;
  }
  return out as TrainingFilter;
}
