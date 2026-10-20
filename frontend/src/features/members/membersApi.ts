import { useInfiniteQuery, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import type { components } from '../../api/schema';

type Schemas = components['schemas'];
export type Member = Schemas['Member'];
export type MemberSummary = Schemas['MemberSummary'];
export type MemberChange = Schemas['MemberChange'];
export type MemberCreateInput = Schemas['MemberCreateInput'];
export type MemberPatchInput = Schemas['MemberPatchInput'];
export type MemberAddress = Schemas['MemberAddress'];
export type MemberContact = Schemas['MemberContact'];
export type MemberAddressInput = Schemas['MemberAddressInput'];
export type MemberContactInput = Schemas['MemberContactInput'];

export interface MemberFilter {
  q?: string;
  membership_status_id?: number;
  limit?: number;
}

export const memberKeys = {
  list: (filter: MemberFilter) => ['members', 'list', filter] as const,
  card: (id: number) => ['members', 'card', id] as const,
  me: ['members', 'me'] as const,
  history: (id: number) => ['members', 'history', id] as const,
  dictionaries: ['member-dictionaries'] as const,
};

/** Membership is active only for the «Активный» status, not for a numeric ID. */
export const ACTIVE_MEMBERSHIP = 'Активный';

export function useMemberDictionaries() {
  return useQuery({ queryKey: memberKeys.dictionaries, queryFn: () => call(api.GET('/api/v1/member-dictionaries')), staleTime: Infinity });
}

export function useMembers(filter: MemberFilter, enabled = true) {
  return useInfiniteQuery({
    queryKey: memberKeys.list(filter),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      call(
        api.GET('/api/v1/members', {
          params: { query: { q: filter.q || undefined, membership_status_id: filter.membership_status_id || undefined, limit: filter.limit, after: pageParam || undefined } },
        }),
      ),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

export function useMember(id: number) {
  return useQuery({ queryKey: memberKeys.card(id), queryFn: () => call(api.GET('/api/v1/members/{memberID}', { params: { path: { memberID: id } } })) });
}

/** Cards of several people at once, e.g. the owners of a dog. */
export function useMembersByID(ids: number[], enabled = true) {
  return useQueries({
    queries: ids.map((id) => ({
      queryKey: memberKeys.card(id),
      enabled,
      queryFn: () => call(api.GET('/api/v1/members/{memberID}', { params: { path: { memberID: id } } })),
    })),
  });
}

export function useOwnMember() {
  return useQuery({ queryKey: memberKeys.me, queryFn: () => call(api.GET('/api/v1/members/me')) });
}

export function useMemberHistory(id: number) {
  return useInfiniteQuery({
    queryKey: memberKeys.history(id),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/members/{memberID}/history', { params: { path: { memberID: id }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

/** Mutations return the whole card with its new version. */
export function useMemberSaved() {
  const client = useQueryClient();
  return (member: Member) => {
    client.setQueryData(memberKeys.card(member.id), member);
    void client.invalidateQueries({ queryKey: ['members', 'list'] });
    void client.invalidateQueries({ queryKey: memberKeys.history(member.id) });
    void client.invalidateQueries({ queryKey: memberKeys.me });
    return member;
  };
}

const path = (memberID: number) => ({ params: { path: { memberID } } });

export const membersApi = {
  create: (body: MemberCreateInput) => call(api.POST('/api/v1/members', { body })),
  patch: (memberID: number, body: MemberPatchInput) => call(api.PATCH('/api/v1/members/{memberID}', { ...path(memberID), body })),
  addContact: (memberID: number, body: MemberContactInput) => call(api.POST('/api/v1/members/{memberID}/contacts', { ...path(memberID), body })),
  replaceContact: (memberID: number, contactID: number, body: MemberContactInput) =>
    call(api.PUT('/api/v1/members/{memberID}/contacts/{contactID}', { params: { path: { memberID, contactID } }, body })),
  deleteContact: (memberID: number, contactID: number, version: number) =>
    call(api.DELETE('/api/v1/members/{memberID}/contacts/{contactID}', { params: { path: { memberID, contactID }, query: { version } } })),
  addAddress: (memberID: number, body: MemberAddressInput) => call(api.POST('/api/v1/members/{memberID}/addresses', { ...path(memberID), body })),
  replaceAddress: (memberID: number, addressID: number, body: MemberAddressInput) =>
    call(api.PUT('/api/v1/members/{memberID}/addresses/{addressID}', { params: { path: { memberID, addressID } }, body })),
  deleteAddress: (memberID: number, addressID: number, version: number) =>
    call(api.DELETE('/api/v1/members/{memberID}/addresses/{addressID}', { params: { path: { memberID, addressID }, query: { version } } })),
};

export function formatAddress(address: Pick<MemberAddress, 'locality' | 'street' | 'building_number' | 'apartment_number'>): string {
  return [address.locality, address.street, address.building_number && `д. ${address.building_number}`, address.apartment_number && `кв. ${address.apartment_number}`]
    .filter(Boolean)
    .join(', ');
}
