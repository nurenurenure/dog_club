import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import type { components } from '../../api/schema';

type Schemas = components['schemas'];
export type Dog = Schemas['Dog'];
export type DogSummary = Schemas['DogSummary'];
export type DogParent = Schemas['DogParent'];
export type DogChange = Schemas['DogChange'];
export type DogCreateInput = Schemas['DogCreateInput'];
export type DogPatchInput = Schemas['DogPatchInput'];
export type DogPedigreeInput = Schemas['DogPedigreeInput'];
export type DogParentInput = Schemas['DogParentInput'];
export type DogDictionaries = Schemas['DogDictionaries'];

export interface DogFilter {
  q?: string;
  breed_id?: number;
  status_id?: number;
  sex_id?: number;
  owner_id?: number;
  limit?: number;
}

export const dogKeys = {
  all: ['dogs'] as const,
  list: (filter: DogFilter) => ['dogs', 'list', filter] as const,
  card: (id: number) => ['dogs', 'card', id] as const,
  history: (id: number) => ['dogs', 'history', id] as const,
  dictionaries: ['dog-dictionaries'] as const,
};

/** Dictionaries change only with migrations: one load per session is enough. */
export function useDogDictionaries(enabled = true) {
  return useQuery({
    queryKey: dogKeys.dictionaries,
    enabled,
    queryFn: () => call(api.GET('/api/v1/dog-dictionaries')),
    staleTime: Infinity,
  });
}

/** Cursor pages of the catalogue; empty filter values are not sent. */
export function useDogs(filter: DogFilter, enabled = true) {
  return useInfiniteQuery({
    queryKey: dogKeys.list(filter),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/dogs', { params: { query: { ...compact(filter), after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

export function useDog(id: number, enabled = true) {
  return useQuery({ queryKey: dogKeys.card(id), enabled, queryFn: () => call(api.GET('/api/v1/dogs/{dogID}', { params: { path: { dogID: id } } })) });
}

export function useDogHistory(id: number) {
  return useInfiniteQuery({
    queryKey: dogKeys.history(id),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/dogs/{dogID}/history', { params: { path: { dogID: id }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

/** Every mutation answers with the whole card: put it in the cache, refresh lists and history. */
export function useDogSaved() {
  const client = useQueryClient();
  return (dog: Dog) => {
    client.setQueryData(dogKeys.card(dog.id), dog);
    void client.invalidateQueries({ queryKey: ['dogs', 'list'] });
    void client.invalidateQueries({ queryKey: dogKeys.history(dog.id) });
    return dog;
  };
}

const path = (dogID: number) => ({ params: { path: { dogID } } });

export const dogsApi = {
  get: (dogID: number) => call(api.GET('/api/v1/dogs/{dogID}', path(dogID))),
  create: (body: DogCreateInput) => call(api.POST('/api/v1/dogs', { body })),
  patch: (dogID: number, body: DogPatchInput) => call(api.PATCH('/api/v1/dogs/{dogID}', { ...path(dogID), body })),
  setOwners: (dogID: number, version: number, owner_ids: number[]) => call(api.PUT('/api/v1/dogs/{dogID}/owners', { ...path(dogID), body: { version, owner_ids } })),
  setPedigree: (dogID: number, body: DogPedigreeInput) => call(api.PUT('/api/v1/dogs/{dogID}/pedigree', { ...path(dogID), body })),
  setRegistration: (dogID: number, version: number, registration_date: string | null, reason: string) =>
    call(api.PUT('/api/v1/dogs/{dogID}/registration', { ...path(dogID), body: { version, registration_date, reason } })),
  recordTitle: (dogID: number, body: Schemas['DogTitleInput']) => call(api.POST('/api/v1/dogs/{dogID}/titles', { ...path(dogID), body })),
  updateTitle: (dogID: number, titleID: number, body: Schemas['DogTitleUpdateInput']) =>
    call(api.PUT('/api/v1/dogs/{dogID}/titles/{titleID}', { params: { path: { dogID, titleID } }, body })),
  removeTitle: (dogID: number, titleID: number, version: number, reason: string) =>
    call(api.DELETE('/api/v1/dogs/{dogID}/titles/{titleID}', { params: { path: { dogID, titleID } }, body: { version, reason } })),
};

function compact(filter: DogFilter) {
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(filter)) {
    if (value !== undefined && value !== '' && value !== 0) out[key] = value;
  }
  return out as DogFilter;
}

/** Only «На учёте» is active (lab 3): inactive dogs get no new records. */
export function isActiveStatus(dictionaries: DogDictionaries | undefined, statusID: number): boolean {
  return dictionaries?.statuses.find((status) => status.id === statusID)?.is_active ?? true;
}
