// Event times are instants with an explicit offset; staff types them as wall-clock
// time of the event's IANA zone. Intl knows the zone rules, so no library is needed.

/** Zones offered in the event form; the event's own zone is added when it is not listed. */
export const commonZones = [
  'Europe/Kaliningrad',
  'Europe/Moscow',
  'Europe/Samara',
  'Asia/Yekaterinburg',
  'Asia/Omsk',
  'Asia/Novosibirsk',
  'Asia/Barnaul',
  'Asia/Krasnoyarsk',
  'Asia/Irkutsk',
  'Asia/Yakutsk',
  'Asia/Vladivostok',
  'Asia/Magadan',
  'Asia/Kamchatka',
  'UTC',
];

const formatters = new Map<string, Intl.DateTimeFormat>();

function formatter(zone: string) {
  let out = formatters.get(zone);
  if (!out) {
    out = new Intl.DateTimeFormat('en-US', {
      timeZone: zone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    formatters.set(zone, out);
  }
  return out;
}

interface Wall {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
}

function wallClock(instant: number, zone: string): Wall & { second: number } {
  const parts = Object.fromEntries(
    formatter(zone)
      .formatToParts(new Date(instant))
      .map((part) => [part.type, part.value]),
  );
  return { year: Number(parts.year), month: Number(parts.month), day: Number(parts.day), hour: Number(parts.hour), minute: Number(parts.minute), second: Number(parts.second) };
}

/** Offset of the zone at an instant, in minutes east of UTC. */
export function zoneOffset(instant: number, zone: string): number {
  const wall = wallClock(instant, zone);
  const asUTC = Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute, wall.second);
  return Math.round((asUTC - Math.floor(instant / 1000) * 1000) / 60_000);
}

export function isZone(zone: string): boolean {
  try {
    formatter(zone);
    return true;
  } catch {
    return false;
  }
}

const pad = (value: number, size = 2) => String(Math.abs(value)).padStart(size, '0');

function offsetText(minutes: number) {
  return `${minutes < 0 ? '-' : '+'}${pad(Math.floor(Math.abs(minutes) / 60))}:${pad(Math.abs(minutes) % 60)}`;
}

/**
 * "2026-11-14T10:00" in the zone → RFC 3339 with that zone's offset. A time skipped by
 * a clock change moves forward by the gap; a repeated hour takes its first occurrence.
 */
export function zonedToRFC3339(local: string, zone: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(local);
  if (!match) return '';
  const [year, month, day, hour, minute] = match.slice(1).map(Number) as [number, number, number, number, number];
  const naive = Date.UTC(year, month - 1, day, hour, minute);
  // The offsets a day before and after cover any single clock change; the earlier valid one wins.
  const before = zoneOffset(naive - 86_400_000, zone);
  const after = zoneOffset(naive + 86_400_000, zone);
  const valid = [before, after].find((offset) => zoneOffset(naive - offset * 60_000, zone) === offset);
  // No valid offset means the time was skipped: the old offset lands past the gap.
  return toZonedRFC3339(naive - (valid ?? before) * 60_000, zone);
}

function toZonedRFC3339(instant: number, zone: string): string {
  const wall = wallClock(instant, zone);
  return `${pad(wall.year, 4)}-${pad(wall.month)}-${pad(wall.day)}T${pad(wall.hour)}:${pad(wall.minute)}:00${offsetText(zoneOffset(instant, zone))}`;
}

/** An instant from the API → the value of a datetime-local input in the zone. */
export function toZonedInput(value: string | null | undefined, zone: string): string {
  if (!value) return '';
  const instant = Date.parse(value);
  if (Number.isNaN(instant)) return '';
  const wall = wallClock(instant, zone);
  return `${pad(wall.year, 4)}-${pad(wall.month)}-${pad(wall.day)}T${pad(wall.hour)}:${pad(wall.minute)}`;
}

/** "14.11.2026, 10:00" as the clock reads at the event. */
export function formatZoned(value: string | null | undefined, zone: string | null): string {
  if (!value) return '—';
  const instant = Date.parse(value);
  if (Number.isNaN(instant)) return value;
  const wall = wallClock(instant, zone || 'UTC');
  return `${pad(wall.day)}.${pad(wall.month)}.${wall.year}, ${pad(wall.hour)}:${pad(wall.minute)}`;
}

/** A period inside one day keeps one date: "14.11.2026, 10:00–18:00". */
export function formatPeriod(start: string | null, end: string | null, zone: string | null): string {
  if (!start || !end) return '—';
  const from = formatZoned(start, zone);
  const to = formatZoned(end, zone);
  const [fromDay] = from.split(', ');
  const [toDay, toTime] = to.split(', ');
  return fromDay === toDay ? `${from}–${toTime}` : `${from} — ${to}`;
}

/** "Europe/Moscow, UTC+03:00": the zone and its offset at that moment. */
export function zoneCaption(zone: string | null, at?: string | null): string {
  const name = zone || 'UTC';
  if (name === 'UTC') return 'UTC';
  const instant = at ? Date.parse(at) : Date.now();
  return `${name}, UTC${offsetText(zoneOffset(Number.isNaN(instant) ? Date.now() : instant, name))}`;
}
