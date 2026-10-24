import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { setSession } from '../../api/session';
import { json, mockFetch, staffCredentials } from '../../test/fetchMock';
import { renderApp } from '../../test/renderApp';

afterEach(() => setSession(null));

describe('access notices', () => {
  it('tells an owner without a member card to ask the administrator and rechecks on demand', async () => {
    let linked = false;
    const owner = { ...staffCredentials, profile: { id: 'p-owner', role: 'owner', enabled: true } };
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(owner),
      'GET /api/v1/auth/me': () => json({ user: linked ? { ...owner.user, club_member_id: 3 } : owner.user, profile: owner.profile }),
    });
    renderApp('/account');
    expect(await screen.findByText('Карточка не привязана')).toBeTruthy();
    expect(screen.getByText(/Обратитесь к администратору клуба/)).toBeTruthy();
    linked = true;
    await userEvent.click(screen.getByRole('button', { name: 'Проверить снова' }));
    await waitFor(() => expect(screen.queryByText('Карточка не привязана')).toBeNull());
  });
});
