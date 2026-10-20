import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { setSession } from '../../api/session';
import { dog, dogDictionaries, member } from '../../test/fixtures';
import { json, mockFetch, problem } from '../../test/fetchMock';
import { credentialsFor, renderApp } from '../../test/renderApp';

afterEach(() => setSession(null));

/** A list row: the card without owners, pedigree, titles and registration. */
const summary = (overrides = {}) => {
  const { owner_ids, pedigree, titles, registration_date, ...rest } = dog(overrides);
  return rest;
};

describe('dogs catalogue', () => {
  it('lists dogs, sends filters to the server and offers registration to staff', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/dogs': () => json({ dogs: [summary(), summary({ id: 8, nickname: 'Ласка', status: { id: 2, name: 'Потеряна' } })], next_after: null }),
    });
    renderApp('/dogs');
    expect(await screen.findByRole('link', { name: 'Бублик' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Зарегистрировать собаку' })).toBeTruthy();
    await userEvent.selectOptions(screen.getByLabelText('Пол'), 'Сука');
    await waitFor(() => expect(calls.some((call) => new URL(call.url).searchParams.get('sex_id') === '2')).toBe(true));
  });

  it('hides changes from the leader', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('leader')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/dogs/7': () => json(dog()),
      'GET /api/v1/dogs/5': () => json(dog({ id: 5, nickname: 'Граф', pedigree: { number: '', issuer: '', father: null, mother: null } })),
    });
    renderApp('/dogs/7');
    expect(await screen.findByRole('heading', { name: 'Бублик' })).toBeTruthy();
    expect(screen.getByText('Известна на 3 поколения')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Граф' })).toBeTruthy();
    expect(screen.queryByRole('link', { name: 'Изменить' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Изменить' })).toBeNull();
  });
});

describe('dog card', () => {
  it('shows an owner co-owners without reading other people', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('owner')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/dogs/7': () => json(dog({ status: { id: 2, name: 'Потеряна' } })),
    });
    renderApp('/dogs/7?tab=owners');
    expect(await screen.findByText('Вы и ещё 1 совладелец.')).toBeTruthy();
    expect(await screen.findByText('Собака не на учёте: новые записи недоступны')).toBeTruthy();
    expect(calls.some((call) => call.url.includes('/members'))).toBe(false);
  });

  it('offers a reload when another user changed the card', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/dogs/7': () => json(dog()),
      'DELETE /api/v1/dogs/7/titles/1': () => problem(409, 'version_conflict', 'Версия устарела.'),
    });
    renderApp('/dogs/7?tab=titles');
    await userEvent.click(await screen.findByRole('button', { name: 'Отметить ошибочной' }));
    const dialog = screen.getByRole('dialog');
    await userEvent.type(within(dialog).getByLabelText('Причина'), 'Титул другой собаки');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Отметить ошибочной' }));
    expect(await within(dialog).findByText('Запись изменил другой пользователь')).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Обновить' })).toBeTruthy();
  });

  it('replaces the pedigree with a mother found among females', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/dog-dictionaries': () => json(dogDictionaries),
      'GET /api/v1/dogs/7': () => json(dog()),
      'GET /api/v1/dogs/5': () => json(dog({ id: 5, nickname: 'Граф' })),
      'GET /api/v1/dogs': () => json({ dogs: [summary({ id: 9, nickname: 'Альма', sex: { id: 2, name: 'Сука' } })], next_after: null }),
      'PUT /api/v1/dogs/7/pedigree': () => json(dog({ version: 5 })),
      'GET /api/v1/members/3': () => json(member()),
    });
    renderApp('/dogs/7');
    await userEvent.click(await screen.findByRole('button', { name: 'Изменить' }));
    const mother = screen.getByRole('group', { name: 'Мать' });
    await userEvent.click(within(mother).getByRole('tab', { name: 'Собака клуба' }));
    await userEvent.type(within(mother).getByLabelText('Найти: мать'), 'Аль');
    await userEvent.click(await within(mother).findByRole('button', { name: /Альма/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Сохранить родословную' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PUT')).toBe(true));
    const search = calls.find((call) => call.method === 'GET' && new URL(call.url).pathname === '/api/v1/dogs')!;
    expect(new URL(search.url).searchParams.get('sex_id')).toBe('2');
    const body = await calls.find((call) => call.method === 'PUT')!.json();
    expect(body).toEqual({
      version: 4,
      number: 'РКФ 1234567',
      issuer: 'РКФ',
      mother: { dog_id: 9 },
      father: { dog_id: 5 },
    });
  });
});
