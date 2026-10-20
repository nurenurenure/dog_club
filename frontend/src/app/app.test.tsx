import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { setSession } from '../api/session';
import { AuthProvider } from '../auth/AuthProvider';
import { json, mockFetch, problem, staffCredentials } from '../test/fetchMock';
import { routes } from './router';

function renderApp(path: string) {
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>,
  );
  return router;
}

afterEach(() => setSession(null));

describe('authentication flow', () => {
  it('sends an anonymous visitor to login and opens the role menu after sign-in', async () => {
    mockFetch({
      'POST /api/v1/auth/refresh': () => problem(401, 'unauthenticated', 'Войдите.'),
      'POST /api/v1/auth/login': () => json(staffCredentials),
    });
    const router = renderApp('/members');
    expect(await screen.findByRole('heading', { name: 'Где собаки становятся чемпионами' })).toBeTruthy();
    await userEvent.type(screen.getByLabelText('Логин'), 'ivanova.m');
    await userEvent.type(screen.getByLabelText('Пароль'), 'a long synthetic passphrase');
    await userEvent.click(screen.getByRole('button', { name: 'Войти' }));
    const nav = (await screen.findAllByRole('navigation', { name: 'Разделы' }))[0]!;
    expect(within(nav).getByRole('link', { name: 'Члены клуба' }).getAttribute('aria-current')).toBe('page');
    expect(router.state.location.pathname).toBe('/members');
  });

  it('restores the session from the refresh cookie and refuses foreign sections', async () => {
    mockFetch({ 'POST /api/v1/auth/refresh': () => json({ ...staffCredentials, profile: { id: 'p-vet', role: 'veterinarian', enabled: true } }) });
    renderApp('/members');
    expect(await screen.findByText('Раздел недоступен профилю «Ветеринар»')).toBeTruthy();
    const nav = screen.getAllByRole('navigation', { name: 'Разделы' })[0]!;
    expect(within(nav).getByRole('link', { name: 'Пациенты' })).toBeTruthy();
    expect(within(nav).queryByRole('link', { name: 'Члены клуба' })).toBeNull();
  });

  it('starts a non-dashboard role on its first section', async () => {
    mockFetch({ 'POST /api/v1/auth/refresh': () => json({ ...staffCredentials, profile: { id: 'p-admin', role: 'admin', enabled: true } }) });
    const router = renderApp('/');
    expect(await screen.findByRole('heading', { name: 'Пользователи' })).toBeTruthy();
    expect(router.state.location.pathname).toBe('/admin/users');
  });
});
