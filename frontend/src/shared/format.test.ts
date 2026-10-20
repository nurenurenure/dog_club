import { describe, expect, it } from 'vitest';
import { formatAge, formatDate, parseMeasure, pluralize } from './format';

describe('format', () => {
  it('shows API dates without time zone shifts', () => {
    expect(formatDate('2023-05-04')).toBe('04.05.2023');
    expect(formatDate(null)).toBe('—');
  });

  it('counts age in months, then in full years', () => {
    expect(formatAge('2026-01-15', '2026-10-14')).toBe('8 месяцев');
    expect(formatAge('2023-05-04', '2026-05-03')).toBe('2 года');
    expect(formatAge('2021-05-04', '2026-05-04')).toBe('5 лет');
  });

  it('declines Russian nouns', () => {
    expect(pluralize(1, 'запись', 'записи', 'записей')).toBe('1 запись');
    expect(pluralize(12, 'запись', 'записи', 'записей')).toBe('12 записей');
    expect(pluralize(22, 'запись', 'записи', 'записей')).toBe('22 записи');
  });

  it('accepts a decimal comma in measurements', () => {
    expect(parseMeasure('55,5')).toBe(55.5);
    expect(parseMeasure(' ')).toBeNull();
    expect(parseMeasure('abc')).toBeNaN();
  });
});
