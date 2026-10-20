import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import type { components } from '../../api/schema';

type Schemas = components['schemas'];
export type Event = Schemas['Event'];
export type EventStatus = Event['status'];
export type EventClass = Schemas['EventClass'];
export type EventSchedule = Schemas['EventSchedule'];
export type Venue = Schemas['EventVenue'];
export type Registration = Schemas['EventRegistration'];
export type Eligibility = Schemas['EventEligibility'];
export type BlockingReason = Eligibility['blocking_reasons'][number];
export type EventResult = Schemas['EventResult'];
export type EventChange = Schemas['EventChange'];
export type EventInput = Schemas['EventInput'];
export type ClassCommand = Schemas['EventClassCommand'];
export type ScheduleCommand = Schemas['EventScheduleCommand'];

export interface EventFilter {
  q?: string;
  status?: EventStatus;
  type_id?: number;
  from?: string;
  to?: string;
}

export const eventKeys = {
  all: ['events'] as const,
  list: (filter: EventFilter) => ['events', 'list', filter] as const,
  event: (id: number) => ['events', 'event', id] as const,
  registrations: (eventID: number) => ['events', 'event', eventID, 'registrations'] as const,
  registration: (eventID: number, id: number) => ['events', 'event', eventID, 'registration', id] as const,
  results: (eventID: number) => ['events', 'event', eventID, 'results'] as const,
  history: (path: string) => ['events', 'history', path] as const,
  venues: ['event-venues'] as const,
  dictionaries: ['event-dictionaries'] as const,
  experts: ['event-experts'] as const,
};

// Lists of one event are short: load every page at once.
async function all<T>(load: (after: number) => Promise<{ records: T[]; next_after: number | null }>): Promise<T[]> {
  const out: T[] = [];
  let after = 0;
  for (let page = 0; page < 20; page += 1) {
    const result = await load(after);
    out.push(...result.records);
    if (result.next_after === null) break;
    after = result.next_after;
  }
  return out;
}

const page = (after: number) => ({ after: after || undefined, limit: 100 });

export function useEvents(filter: EventFilter) {
  return useInfiniteQuery({
    queryKey: eventKeys.list(filter),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/events', { params: { query: { ...compact(filter), after: pageParam || undefined, limit: 50 } } })),
    getNextPageParam: (result) => result.next_after ?? undefined,
  });
}

export function useEvent(id: number) {
  return useQuery({ queryKey: eventKeys.event(id), queryFn: () => call(api.GET('/api/v1/events/{eventID}', { params: { path: { eventID: id } } })) });
}

export function useRegistrations(eventID: number, enabled = true) {
  return useQuery({
    queryKey: eventKeys.registrations(eventID),
    enabled,
    queryFn: () => all((after) => call(api.GET('/api/v1/events/{eventID}/registrations', { params: { path: { eventID }, query: page(after) } }))),
  });
}

export function useRegistration(eventID: number, id: number) {
  return useQuery({
    queryKey: eventKeys.registration(eventID, id),
    queryFn: () => call(api.GET('/api/v1/events/{eventID}/registrations/{registrationID}', { params: { path: { eventID, registrationID: id } } })),
  });
}

export function useResults(eventID: number, enabled = true) {
  return useQuery({
    queryKey: eventKeys.results(eventID),
    enabled,
    queryFn: () => all((after) => call(api.GET('/api/v1/events/{eventID}/results', { params: { path: { eventID }, query: page(after) } }))),
  });
}

/** History of an event, registration, result or venue; the path names which. */
export type HistoryTarget =
  | { kind: 'event'; eventID: number }
  | { kind: 'registration'; eventID: number; id: number }
  | { kind: 'result'; eventID: number; id: number }
  | { kind: 'venue'; id: number };

export function useEventHistory(target: HistoryTarget) {
  const key = target.kind === 'venue' ? `venue/${target.id}` : target.kind === 'event' ? `event/${target.eventID}` : `${target.kind}/${target.eventID}/${target.id}`;
  return useInfiniteQuery({
    queryKey: eventKeys.history(key),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const query = { after: pageParam || undefined };
      if (target.kind === 'venue') return call(api.GET('/api/v1/event-venues/{venueID}/history', { params: { path: { venueID: target.id }, query } }));
      if (target.kind === 'registration')
        return call(
          api.GET('/api/v1/events/{eventID}/registrations/{registrationID}/history', { params: { path: { eventID: target.eventID, registrationID: target.id }, query } }),
        );
      if (target.kind === 'result')
        return call(api.GET('/api/v1/events/{eventID}/results/{resultID}/history', { params: { path: { eventID: target.eventID, resultID: target.id }, query } }));
      return call(api.GET('/api/v1/events/{eventID}/history', { params: { path: { eventID: target.eventID }, query } }));
    },
    getNextPageParam: (result) => result.next_after ?? undefined,
  });
}

export function useVenues(enabled = true) {
  return useQuery({ queryKey: eventKeys.venues, enabled, queryFn: () => all((after) => call(api.GET('/api/v1/event-venues', { params: { query: page(after) } }))) });
}

export function useEventDictionaries() {
  return useQuery({ queryKey: eventKeys.dictionaries, staleTime: Infinity, queryFn: () => call(api.GET('/api/v1/event-dictionaries')) });
}

/** Dogs that staff may enter; the dog card then gives its current owners. */
export function useEventDogs(q: string) {
  return useQuery({
    queryKey: ['events', 'dogs', q],
    enabled: q.length > 0,
    queryFn: () => call(api.GET('/api/v1/events/dogs', { params: { query: { q, limit: 10 } } })),
  });
}

/** Display names of expert profiles: staff and leader only. */
export function useEventExperts(enabled: boolean) {
  return useQuery({ queryKey: eventKeys.experts, enabled, staleTime: 60_000, queryFn: () => call(api.GET('/api/v1/event-experts')) });
}

/** One change touches the event, its registrations and results: refresh them all. */
export function useEventsSaved() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: eventKeys.all });
}

export function useVenuesSaved() {
  const client = useQueryClient();
  return () => Promise.all([client.invalidateQueries({ queryKey: eventKeys.venues }), client.invalidateQueries({ queryKey: eventKeys.all })]);
}

const ev = (eventID: number) => ({ params: { path: { eventID } } });
const reg = (eventID: number, registrationID: number) => ({ params: { path: { eventID, registrationID } } });
const res = (eventID: number, resultID: number) => ({ params: { path: { eventID, resultID } } });

export const eventsApi = {
  createVenue: (body: Schemas['EventVenueInput']) => call(api.POST('/api/v1/event-venues', { body })),
  updateVenue: (venueID: number, body: Schemas['EventVenueUpdate']) => call(api.PUT('/api/v1/event-venues/{venueID}', { params: { path: { venueID } }, body })),
  create: (body: EventInput) => call(api.POST('/api/v1/events', { body })),
  update: (eventID: number, body: Schemas['EventUpdate']) => call(api.PUT('/api/v1/events/{eventID}', { ...ev(eventID), body })),
  setState: (eventID: number, body: Schemas['EventStateInput']) => call(api.POST('/api/v1/events/{eventID}/state', { ...ev(eventID), body })),
  setExperts: (eventID: number, body: Schemas['EventExpertsInput']) => call(api.PUT('/api/v1/events/{eventID}/experts', { ...ev(eventID), body })),
  addClass: (eventID: number, body: ClassCommand) => call(api.POST('/api/v1/events/{eventID}/classes', { ...ev(eventID), body })),
  updateClass: (eventID: number, classID: number, body: ClassCommand) =>
    call(api.PUT('/api/v1/events/{eventID}/classes/{classID}', { params: { path: { eventID, classID } }, body })),
  addSlot: (eventID: number, body: ScheduleCommand) => call(api.POST('/api/v1/events/{eventID}/schedule', { ...ev(eventID), body })),
  updateSlot: (eventID: number, scheduleID: number, body: ScheduleCommand) =>
    call(api.PUT('/api/v1/events/{eventID}/schedule/{scheduleID}', { params: { path: { eventID, scheduleID } }, body })),
  register: (eventID: number, body: Schemas['EventRegistrationInput']) => call(api.POST('/api/v1/events/{eventID}/registrations', { ...ev(eventID), body })),
  updateRegistration: (eventID: number, id: number, body: Schemas['EventRegistrationUpdate']) =>
    call(api.PUT('/api/v1/events/{eventID}/registrations/{registrationID}', { ...reg(eventID, id), body })),
  attend: (eventID: number, id: number, version: number, reason: string) =>
    call(api.POST('/api/v1/events/{eventID}/registrations/{registrationID}/attend', { ...reg(eventID, id), body: { version, reason } })),
  cancel: (eventID: number, id: number, version: number, reason: string) =>
    call(api.POST('/api/v1/events/{eventID}/registrations/{registrationID}/cancel', { ...reg(eventID, id), body: { version, reason } })),
  qualify: (eventID: number, id: number, body: Schemas['EventQualificationInput']) =>
    call(api.POST('/api/v1/events/{eventID}/registrations/{registrationID}/qualify', { ...reg(eventID, id), body })),
  recordResult: (eventID: number, id: number, body: Schemas['EventResultCreate']) =>
    call(api.POST('/api/v1/events/{eventID}/registrations/{registrationID}/result', { ...reg(eventID, id), body })),
  correctResult: (eventID: number, id: number, body: Schemas['EventResultUpdate']) => call(api.PUT('/api/v1/events/{eventID}/results/{resultID}', { ...res(eventID, id), body })),
  voidResult: (eventID: number, id: number, version: number, reason: string) =>
    call(api.POST('/api/v1/events/{eventID}/results/{resultID}/void', { ...res(eventID, id), body: { version, reason } })),
};

function compact(filter: EventFilter) {
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(filter)) {
    if (value !== undefined && value !== '' && value !== 0) out[key] = value;
  }
  return out as EventFilter;
}
