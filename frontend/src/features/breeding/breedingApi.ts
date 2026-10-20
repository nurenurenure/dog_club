import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import { ApiError } from '../../api/problem';
import type { components } from '../../api/schema';
import { useSession } from '../../auth/AuthProvider';

type Schemas = components['schemas'];
export type BreedingRequest = Schemas['BreedingRequest'];
export type Assessment = Schemas['BreedingAssessment'];
export type Animal = Schemas['BreedingAnimal'];
export type Decision = Schemas['BreedingDecision'];
export type Mating = Schemas['BreedingMating'];
export type Litter = Schemas['BreedingLitter'];
export type BreedingChange = Schemas['BreedingChange'];
export type DogSummary = Schemas['DogSummary'];

export const breedingKeys = {
  all: ['breeding'] as const,
  requests: (dogID?: number) => ['breeding', 'requests', dogID ?? 0] as const,
  request: (id: number) => ['breeding', 'request', id] as const,
  check: (id: number) => ['breeding', 'request', id, 'check'] as const,
  decisions: (id: number) => ['breeding', 'request', id, 'decisions'] as const,
  matings: (dogID?: number) => ['breeding', 'matings', dogID ?? 0] as const,
  mating: (id: number) => ['breeding', 'mating', id] as const,
  litters: (dogID?: number) => ['breeding', 'litters', dogID ?? 0] as const,
  litter: (id: number) => ['breeding', 'litter', id] as const,
  history: (entity: Entity, id: number) => ['breeding', 'history', entity, id] as const,
  candidates: (dogID: number) => ['breeding', 'candidates', dogID] as const,
  dog: (id: number) => ['breeding', 'dog', id] as const,
};

type Entity = 'breeding-requests' | 'breedings' | 'litters';

// Breeding lists are short: load every page and show the newest first.
async function all<T>(load: (after: number) => Promise<{ records: T[]; next_after: number | null }>): Promise<T[]> {
  const out: T[] = [];
  let after = 0;
  for (let page = 0; page < 20; page += 1) {
    const result = await load(after);
    out.push(...result.records);
    if (result.next_after === null) break;
    after = result.next_after;
  }
  return out.reverse();
}

const listQuery = (dogID: number | undefined, after: number) => ({ params: { query: { dog_id: dogID, after: after || undefined, limit: 100 } } });

export function useRequests(dogID?: number) {
  return useQuery({ queryKey: breedingKeys.requests(dogID), queryFn: () => all((after) => call(api.GET('/api/v1/breeding-requests', listQuery(dogID, after)))) });
}

export function useMatings(dogID?: number) {
  return useQuery({ queryKey: breedingKeys.matings(dogID), queryFn: () => all((after) => call(api.GET('/api/v1/breedings', listQuery(dogID, after)))) });
}

export function useLitters(dogID?: number) {
  return useQuery({ queryKey: breedingKeys.litters(dogID), queryFn: () => all((after) => call(api.GET('/api/v1/litters', listQuery(dogID, after)))) });
}

const byID = (id: number) => ({ params: { path: { id } } });

export function useRequest(id: number) {
  return useQuery({ queryKey: breedingKeys.request(id), queryFn: () => call(api.GET('/api/v1/breeding-requests/{id}', byID(id))) });
}

/** The current pair check: recalculated on every read, unlike decision snapshots. */
export function usePairCheck(id: number, enabled: boolean) {
  return useQuery({ queryKey: breedingKeys.check(id), enabled, queryFn: () => call(api.GET('/api/v1/breeding-requests/{id}/check', byID(id))) });
}

export function useDecisions(id: number, enabled = true) {
  return useQuery({
    queryKey: breedingKeys.decisions(id),
    enabled,
    queryFn: () => all((after) => call(api.GET('/api/v1/breeding-requests/{id}/decisions', { params: { path: { id }, query: { after: after || undefined, limit: 100 } } }))),
  });
}

export function useMating(id: number) {
  return useQuery({ queryKey: breedingKeys.mating(id), queryFn: () => call(api.GET('/api/v1/breedings/{id}', byID(id))) });
}

export function useLitter(id: number) {
  return useQuery({ queryKey: breedingKeys.litter(id), queryFn: () => call(api.GET('/api/v1/litters/{id}', byID(id))) });
}

export function useBreedingHistory(entity: Entity, id: number) {
  return useInfiniteQuery({
    queryKey: breedingKeys.history(entity, id),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = { params: { path: { id }, query: { after: pageParam || undefined } } };
      if (entity === 'breedings') return call(api.GET('/api/v1/breedings/{id}/history', params));
      if (entity === 'litters') return call(api.GET('/api/v1/litters/{id}/history', params));
      return call(api.GET('/api/v1/breeding-requests/{id}/history', params));
    },
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

export function useMateCandidates(dogID: number) {
  return useQuery({
    queryKey: breedingKeys.candidates(dogID),
    queryFn: () => call(api.GET('/api/v1/breeding/dogs/{dogID}/mate-candidates', { params: { path: { dogID } } })),
  });
}

/**
 * Breeding records carry only dog IDs. Staff, leader and expert read short cards through
 * the breeding search ordered by ID (after=id-1, limit=1); an owner reads only own dogs,
 * so a partner stays «собака №ID».
 */
export function useBreedingDog(id: number) {
  const { profile } = useSession();
  const owner = profile.role === 'owner';
  return useQuery({
    queryKey: breedingKeys.dog(id),
    staleTime: 60_000,
    queryFn: async (): Promise<DogSummary | null> => {
      if (owner) {
        try {
          return await call(api.GET('/api/v1/dogs/{dogID}', { params: { path: { dogID: id } } }));
        } catch (error) {
          if (error instanceof ApiError && error.status === 404) return null;
          throw error;
        }
      }
      const page = await call(api.GET('/api/v1/breeding/dogs', { params: { query: { after: id - 1, limit: 1 } } }));
      const dog = page.dogs[0];
      return dog && dog.id === id ? dog : null;
    },
  });
}

/** Records of one process change together: refresh the whole breeding cache. */
export function useBreedingSaved() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: breedingKeys.all });
}

export const breedingApi = {
  createRequest: (body: Schemas['BreedingRequestInput']) => call(api.POST('/api/v1/breeding-requests', { body })),
  updateRequest: (id: number, body: Schemas['BreedingRequestUpdate']) => call(api.PUT('/api/v1/breeding-requests/{id}', { ...byID(id), body })),
  cancel: (id: number, version: number, reason: string) => call(api.POST('/api/v1/breeding-requests/{id}/cancel', { ...byID(id), body: { version, reason } })),
  decide: (id: number, body: Schemas['BreedingDecisionInput']) => call(api.POST('/api/v1/breeding-requests/{id}/decisions', { ...byID(id), body })),
  perform: (id: number, version: number, date: string, reason: string) =>
    call(api.POST('/api/v1/breeding-requests/{id}/perform', { ...byID(id), body: { version, date, reason } })),
  correctMating: (id: number, body: Schemas['BreedingMatingUpdate']) => call(api.PUT('/api/v1/breedings/{id}', { ...byID(id), body })),
  createLitter: (body: Schemas['BreedingLitterInput']) => call(api.POST('/api/v1/litters', { body })),
  correctLitter: (id: number, body: Schemas['BreedingLitterUpdate']) => call(api.PUT('/api/v1/litters/{id}', { ...byID(id), body })),
  registerPuppy: (id: number, body: Schemas['BreedingPuppyInput']) => call(api.POST('/api/v1/litters/{id}/puppies', { ...byID(id), body })),
  linkPuppy: (id: number, body: Schemas['BreedingLinkInput']) => call(api.POST('/api/v1/litters/{id}/puppies/link', { ...byID(id), body })),
  unlinkPuppy: (id: number, dogID: number, version: number, reason: string) =>
    call(api.POST('/api/v1/litters/{id}/puppies/{dogID}/unlink', { params: { path: { id, dogID } }, body: { version, reason } })),
};
