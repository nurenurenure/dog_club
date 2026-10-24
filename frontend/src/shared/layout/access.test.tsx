import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { setSession } from '../../api/session';
import { homePath } from '../../app/navigation';
import { json, mockFetch, problem, staffCredentials } from '../../test/fetchMock';
import { credentialsFor, renderApp } from '../../test/renderApp';

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

  it('offers another profile once the current one is disabled during the session', async () => {
    const vet = { id: 'p-vet', role: 'veterinarian', enabled: true } as const;
    const { calls } = mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/member-dictionaries': () => problem(403, 'forbidden', 'Для этого действия недостаточно прав или профиль отключён.'),
      'GET /api/v1/members': () => problem(403, 'forbidden', 'Для этого действия недостаточно прав или профиль отключён.'),
      'GET /api/v1/auth/me': () => json({ user: staffCredentials.user, profile: { ...staffCredentials.profile, enabled: false } }),
      'GET /api/v1/auth/profiles': () => json({ profiles: [{ ...staffCredentials.profile, enabled: false }, vet] }),
      'POST /api/v1/auth/switch-profile': () => json({ ...staffCredentials, profile: vet }),
    });
    const router = renderApp('/members');
    expect(await screen.findByText('Профиль «Сотрудник клуба» отключён администратором')).toBeTruthy();
    // Many failed requests share one recheck of the account.
    expect(calls.filter((call) => new URL(call.url).pathname === '/api/v1/auth/me')).toHaveLength(1);
    await userEvent.click(await screen.findByRole('button', { name: 'Ветеринар' }));
    await waitFor(() => expect(router.state.location.pathname).toBe(homePath('veterinarian')));
    expect(screen.queryByText(/отключён администратором/)).toBeNull();
  });

  it('keeps the page when a refusal is about the action, not the profile', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => json(credentialsFor('staff')),
      'GET /api/v1/member-dictionaries': () => json({ address_types: [], contact_types: [], membership_statuses: [] }),
      'GET /api/v1/members': () => problem(403, 'forbidden', 'Для этого действия недостаточно прав или профиль отключён.'),
      'GET /api/v1/auth/me': () => json({ user: staffCredentials.user, profile: staffCredentials.profile }),
    });
    renderApp('/members');
    expect(await screen.findByText(/недостаточно прав/)).toBeTruthy();
    expect(screen.queryByText(/отключён администратором/)).toBeNull();
  });
});
