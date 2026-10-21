import { screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { setSession } from '../../api/session';
import { dog, member } from '../../test/fixtures';
import { json, mockFetch, problem } from '../../test/fetchMock';
import { credentialsFor, renderApp } from '../../test/renderApp';

afterEach(() => setSession(null));

const empty = () => json({ records: [], next_after: null });
const { owner_ids, pedigree, titles, registration_date, ...summary } = dog();

describe('home', () => {
  it('shows an owner the dogs with their admission to events', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('owner')),
      'GET /api/v1/members/me': () => json(member()),
      'GET /api/v1/dogs': () => json({ dogs: [summary], next_after: null }),
      'GET /api/v1/dogs/7/medical-status': () =>
        json({
          dog_id: 7,
          as_of: '2026-10-20',
          events: { decision: 'allowed', valid_until: '2026-12-31', clearance_id: 1, reasons: [] },
          breeding: { decision: 'not_assessed', valid_until: null, clearance_id: null, reasons: [] },
        }),
      'GET /api/v1/documents': empty,
      'GET /api/v1/events': empty,
    });
    renderApp('/');
    expect(await screen.findByText('Бублик')).toBeTruthy();
    expect(await screen.findByText('Мероприятия: допущена до 31.12.2026')).toBeTruthy();
    expect(screen.getByText('Мария Иванова')).toBeTruthy();
    expect(await screen.findByText('Ближайших мероприятий нет')).toBeTruthy();
    expect(await screen.findByText('Документов пока нет')).toBeTruthy();
  });

  it('tells an owner with an inactive membership why the club is closed', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('owner')),
      'GET /api/v1/members/me': () => problem(403, 'forbidden', 'Членство неактивно.'),
      'GET /api/v1/dogs': () => problem(403, 'forbidden', 'Членство неактивно.'),
      'GET /api/v1/documents': () => problem(403, 'forbidden', 'Членство неактивно.'),
    });
    renderApp('/');
    expect(await screen.findByText(/Членство неактивно/)).toBeTruthy();
  });

  it('gives staff the overview of totals, waiting requests and gaps', async () => {
    const report = {
      period: { from: '2026-10-20', to: '2026-10-20' },
      generated_at: '2026-10-20T10:00:00Z',
      methodology: '',
      activity: { members_joined: 0, dogs_registered: 0, events_completed: 0, events_by_type: [], matings_performed: 0, litters_born: 0, puppies_born: 0 },
      current: { member_cards: 11, active_members: 10, dog_cards: 20, dogs_on_register: 17 },
      data_quality: { members_without_membership_date: 0, dogs_without_registration_date: 17, performed_matings_without_date: 0, events_without_period: 0 },
    };
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/reports/club-activity': () => json(report),
      'GET /api/v1/breeding-requests': () =>
        json({
          records: [
            { id: 3, female_dog_id: 9, male_dog_id: 5, planned_date: '2026-11-01', expected_birth_date: null, comment: '', status: 'pending', assessment: null, version: 1 },
          ],
          next_after: null,
        }),
      'GET /api/v1/breeding/dogs': () => json({ dogs: [], next_after: null }),
      'GET /api/v1/events': empty,
    });
    renderApp('/');
    expect(await screen.findByText('Заявка 3')).toBeTruthy();
    expect(await screen.findByText('Собак без подтверждённой даты регистрации: 17')).toBeTruthy();
    expect(screen.getByText('на учёте: 17')).toBeTruthy();
    // The upcoming list asks once: a moment taken on every render used to refetch endlessly.
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(calls.filter((call) => new URL(call.url).pathname === '/api/v1/events')).toHaveLength(1);
  });
});
