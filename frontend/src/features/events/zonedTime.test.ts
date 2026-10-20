import { describe, expect, it } from 'vitest';
import { formatPeriod, formatZoned, isZone, toZonedInput, zonedToRFC3339 } from './zonedTime';

describe('event time zones', () => {
  it('writes wall-clock time with the offset of the event zone', () => {
    expect(zonedToRFC3339('2026-11-14T10:00', 'Europe/Moscow')).toBe('2026-11-14T10:00:00+03:00');
    expect(zonedToRFC3339('2026-11-14T10:00', 'Asia/Vladivostok')).toBe('2026-11-14T10:00:00+10:00');
    expect(zonedToRFC3339('2026-11-14T10:00', 'UTC')).toBe('2026-11-14T10:00:00+00:00');
    expect(zonedToRFC3339('14.11.2026', 'UTC')).toBe('');
  });

  it('follows a clock change: summer and winter offsets, skipped and repeated hours', () => {
    expect(zonedToRFC3339('2026-07-01T12:00', 'Europe/Berlin')).toBe('2026-07-01T12:00:00+02:00');
    expect(zonedToRFC3339('2026-03-29T12:00', 'Europe/Berlin')).toBe('2026-03-29T12:00:00+02:00');
    expect(zonedToRFC3339('2026-03-29T01:30', 'Europe/Berlin')).toBe('2026-03-29T01:30:00+01:00');
    // 02:30 does not exist on 29 March: the clock jumps from 02:00 to 03:00.
    expect(zonedToRFC3339('2026-03-29T02:30', 'Europe/Berlin')).toBe('2026-03-29T03:30:00+02:00');
    // 02:30 happens twice on 25 October: the first one is still summer time.
    expect(zonedToRFC3339('2026-10-25T02:30', 'Europe/Berlin')).toBe('2026-10-25T02:30:00+02:00');
    expect(zonedToRFC3339('2026-10-25T12:00', 'Europe/Berlin')).toBe('2026-10-25T12:00:00+01:00');
  });

  it('reads an instant back as the clock reads at the event', () => {
    expect(toZonedInput('2026-11-14T07:00:00Z', 'Europe/Moscow')).toBe('2026-11-14T10:00');
    expect(toZonedInput('2026-11-14T23:30:00Z', 'Asia/Vladivostok')).toBe('2026-11-15T09:30');
    expect(formatZoned('2026-11-14T07:00:00Z', 'Europe/Moscow')).toBe('14.11.2026, 10:00');
    expect(formatZoned('2026-11-14T07:00:00Z', null)).toBe('14.11.2026, 07:00');
    expect(formatPeriod('2026-11-14T07:00:00Z', '2026-11-14T15:00:00Z', 'Europe/Moscow')).toBe('14.11.2026, 10:00–18:00');
    expect(formatPeriod('2026-11-14T07:00:00Z', '2026-11-15T15:00:00Z', 'Europe/Moscow')).toBe('14.11.2026, 10:00 — 15.11.2026, 18:00');
  });

  it('tells a real zone name from a typo', () => {
    expect(isZone('Europe/Moscow')).toBe(true);
    expect(isZone('Europe/Mosсow')).toBe(false);
  });
});
