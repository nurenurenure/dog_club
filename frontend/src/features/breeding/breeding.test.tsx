import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import type { components } from '../../api/schema';
import { setSession } from '../../api/session';
import { dog, dogDictionaries } from '../../test/fixtures';
import { json, mockFetch } from '../../test/fetchMock';
import { credentialsFor, renderApp } from '../../test/renderApp';

type Schemas = components['schemas'];

afterEach(() => setSession(null));

const { owner_ids, pedigree, titles, registration_date, ...male } = dog({ id: 5, nickname: 'Ричи' });
const female = { ...male, id: 9, nickname: 'Альма', sex: { id: 2, name: 'Сука' } };

/** /breeding/dogs?after=id-1&limit=1 answers with that dog. */
const breedingDogs = (request: Request) => {
  const after = Number(new URL(request.url).searchParams.get('after'));
  const found = [female, male].filter((item) => item.id === after + 1);
  return json({ dogs: found, next_after: null });
};

function animal(base: typeof male): Schemas['BreedingAnimal'] {
  return { ...base, pedigree: { number: '', issuer: '', father: null, mother: null }, ancestor_ids: [], ancestry_incomplete: false, pedigree_known_3_generations: true };
}

const health: Schemas['BreedingHealth'] = { as_of: '2026-10-20', admission: { decision: 'allowed', valid_until: '2027-01-01', clearance_id: 1, reasons: [] }, findings: [] };

const request: Schemas['BreedingRequest'] = {
  id: 3,
  female_dog_id: 9,
  male_dog_id: 5,
  planned_date: '2026-11-01',
  expected_birth_date: null,
  comment: '',
  status: 'pending',
  assessment: null,
  version: 1,
};

const assessment: Schemas['BreedingAssessment'] = {
  as_of: '2026-10-20',
  blocking_reasons: ['shared_parent'],
  warnings: ['known_relationship'],
  common_ancestor_ids: [],
  earliest_birth_date: null,
  interval_days: 365,
  female: animal(female),
  male: animal(male),
  female_health: health,
  male_health: health,
};

const empty = () => json({ records: [], next_after: null });

describe('breeding', () => {
  it('keeps approval closed while the pair has a blocking reason', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('expert')),
      'GET /api/v1/breeding-requests/3': () => json(request),
      'GET /api/v1/breeding-requests/3/check': () => json(assessment),
      'GET /api/v1/breeding-requests/3/decisions': empty,
      'GET /api/v1/breeding-requests/3/history': empty,
      'GET /api/v1/breedings': empty,
      'GET /api/v1/breeding/dogs': breedingDogs,
    });
    renderApp('/breeding/requests/3');
    expect(await screen.findByText('Общий родитель')).toBeTruthy();
    expect(screen.getByText('Дальнее родство')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Одобрить' }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText('Одобрение недоступно, пока есть блокирующие причины.')).toBeTruthy();
  });

  it('files a request for a suggested pair', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/breeding/dogs': breedingDogs,
      'POST /api/v1/breeding-requests': () => json(request, 201),
      'GET /api/v1/breeding-requests/3': () => json(request),
      'GET /api/v1/breeding-requests/3/check': () => json(assessment),
      'GET /api/v1/breeding-requests/3/decisions': empty,
      'GET /api/v1/breeding-requests/3/history': empty,
      'GET /api/v1/breedings': empty,
    });
    const router = renderApp('/breeding/requests/new?female=9&male=5');
    expect(await screen.findByText('Альма')).toBeTruthy();
    await userEvent.type(screen.getByLabelText('Планируемая дата вязки'), '2026-11-01');
    await userEvent.click(screen.getByRole('button', { name: 'Подать заявку' }));
    await waitFor(() => expect(router.state.location.pathname).toBe('/breeding/requests/3'));
    const body = await calls.find((call) => call.method === 'POST' && call.url.endsWith('/breeding-requests'))!.json();
    expect(body).toEqual({ female_dog_id: 9, male_dog_id: 5, planned_date: '2026-11-01', expected_birth_date: null, comment: '' });
  });

  it('explains an empty list of mate candidates', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('leader')),
      'GET /api/v1/breeding/dogs': breedingDogs,
      'GET /api/v1/breeding/dogs/9/mate-candidates': () => json({ dog_id: 9, as_of: '2026-10-20', candidates: [], truncated: true }),
    });
    renderApp('/breeding/candidates/9');
    expect(await screen.findByText('Подходящих собак нет')).toBeTruthy();
    expect(screen.getByText(/Проверены первые 500 собак/)).toBeTruthy();
    expect(screen.queryByRole('link', { name: 'Создать заявку' })).toBeNull();
  });

  it('shows an owner the records of their dog without the pair check', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('owner')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/dogs/9': () => json({ ...dog({ id: 9, nickname: 'Альма' }) }),
      'GET /api/v1/dogs/5': () => json({ type: 'about:blank', title: 'Нет', status: 404, code: 'not_found', detail: 'Не найдено.', request_id: 'r' }, 404),
      'GET /api/v1/breeding-requests': () => json({ records: [request], next_after: null }),
      'GET /api/v1/breedings': empty,
      'GET /api/v1/litters': empty,
    });
    renderApp('/dogs/9?tab=breeding');
    expect(await screen.findByText('Заявка 3')).toBeTruthy();
    expect(await screen.findByText('собака №5')).toBeTruthy();
    expect(screen.queryByRole('link', { name: 'Заявка 3' })).toBeNull();
    expect(screen.queryByRole('link', { name: 'Подобрать пару' })).toBeNull();
    expect(calls.some((call) => call.url.includes('/check'))).toBe(false);
  });
});
