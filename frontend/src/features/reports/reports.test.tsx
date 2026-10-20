import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import type { components } from '../../api/schema';
import { setSession } from '../../api/session';
import { json, mockFetch } from '../../test/fetchMock';
import { credentialsFor, renderApp } from '../../test/renderApp';

afterEach(() => setSession(null));

const report: components['schemas']['ClubActivityReport'] = {
  period: { from: '2026-01-01', to: '2026-10-20' },
  generated_at: '2026-10-20T10:00:00Z',
  methodology: 'Считаются только записанные факты.',
  activity: {
    members_joined: 0,
    dogs_registered: 0,
    events_completed: 0,
    events_by_type: [
      { type_id: 1, type_name: 'Выставка', count: 0 },
      { type_id: 6, type_name: 'Семинар', count: 0 },
    ],
    matings_performed: 0,
    litters_born: 0,
    puppies_born: 0,
  },
  current: { member_cards: 12, active_members: 10, dog_cards: 15, dogs_on_register: 13 },
  data_quality: { members_without_membership_date: 0, dogs_without_registration_date: 2, performed_matings_without_date: 0, events_without_period: 1 },
};

describe('reports', () => {
  it('asks for the period explicitly and keeps zero counts visible', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('leader')),
      'GET /api/v1/reports/club-activity': () => json(report),
    });
    renderApp('/reports');
    const from = (await screen.findByLabelText('С даты')) as HTMLInputElement;
    await userEvent.clear(from);
    await userEvent.type(from, '2026-01-01');
    const to = screen.getByLabelText('По дату') as HTMLInputElement;
    await userEvent.clear(to);
    await userEvent.type(to, '2026-10-20');
    expect(calls.some((call) => call.url.includes('/reports/'))).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Сформировать' }));
    expect(await screen.findByText('Семинар')).toBeTruthy();
    const url = new URL(calls.find((call) => call.url.includes('/reports/club-activity'))!.url);
    expect(Object.fromEntries(url.searchParams)).toEqual({ from: '2026-01-01', to: '2026-10-20' });
    expect(screen.getByText('Собаки без даты регистрации').parentElement?.textContent).toContain('2');
    expect(screen.getByText('Вступили в клуб').parentElement?.textContent).toContain('0');
    expect(screen.getByRole('button', { name: /CSV/ })).toBeTruthy();
  });

  it('keeps the report away from other profiles', async () => {
    mockFetch({ 'POST /api/v1/auth/refresh': () => json(credentialsFor('expert')) });
    renderApp('/reports');
    expect(await screen.findByText('Нет доступа')).toBeTruthy();
  });
});
