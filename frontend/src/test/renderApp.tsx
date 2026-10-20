import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { routes } from '../app/router';
import type { Role } from '../api/session';
import { AuthProvider } from '../auth/AuthProvider';
import { staffCredentials } from './fetchMock';

/** The whole app at `path`, with retries off so failures show at once. */
export function renderApp(path: string) {
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

export function credentialsFor(role: Role) {
  return { ...staffCredentials, profile: { id: `p-${role}`, role, enabled: true } };
}
