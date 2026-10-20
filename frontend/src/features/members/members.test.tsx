import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { setSession } from '../../api/session';
import { member, memberDictionaries } from '../../test/fixtures';
import { json, mockFetch, problem } from '../../test/fetchMock';
import { credentialsFor, renderApp } from '../../test/renderApp';

afterEach(() => setSession(null));

describe('members', () => {
  it('adds a contact with the current card version', async () => {
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/member-dictionaries': () => json(memberDictionaries),
      'GET /api/v1/members/3': () => json(member()),
      'POST /api/v1/members/3/contacts': () => json(member({ version: 3 })),
    });
    renderApp('/members/3');
    await userEvent.click(await screen.findByRole('button', { name: 'Добавить контакт' }));
    const dialog = screen.getByRole('dialog');
    await userEvent.selectOptions(within(dialog).getByLabelText('Тип'), 'Email');
    await userEvent.type(within(dialog).getByLabelText('Значение'), 'maria@example.test');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.url.endsWith('/contacts'))).toBe(true));
    const body = await calls.find((call) => call.method === 'POST' && call.url.endsWith('/contacts'))!.json();
    expect(body).toEqual({ version: 2, type_id: 2, value: 'maria@example.test' });
  });

  it('shows an owner their card read only', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('owner')),
      'GET /api/v1/members/me': () => json(member()),
    });
    renderApp('/me');
    expect(await screen.findByRole('heading', { name: 'Мария Иванова' })).toBeTruthy();
    expect(screen.getByText('+7 900 000-00-00')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Добавить контакт' })).toBeNull();
    expect(screen.queryByRole('link', { name: 'Изменить' })).toBeNull();
  });

  it('explains an inactive membership instead of an empty card', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('owner')),
      'GET /api/v1/members/me': () => problem(403, 'forbidden', 'Нет доступа.'),
    });
    renderApp('/me');
    expect(await screen.findByText('Членство неактивно')).toBeTruthy();
  });

  it('keeps registration screens for staff only', async () => {
    mockFetch({ 'POST /api/v1/auth/refresh': () => json(credentialsFor('leader')), 'GET /api/v1/member-dictionaries': () => json(memberDictionaries) });
    renderApp('/members/new');
    expect(await screen.findByText('Профиль «Руководитель клуба» не меняет записи')).toBeTruthy();
  });
});
