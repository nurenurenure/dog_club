import { Navigate, Outlet, useLocation, type RouteObject } from 'react-router';
import { useAuth } from '../auth/AuthProvider';
import { AccountPage } from '../features/account/AccountPage';
import { DogsRoutes } from '../features/dogs/DogsRoutes';
import { OwnCardPage } from '../features/members/MemberPage';
import { MembersRoutes } from '../features/members/MembersRoutes';
import { PatientsRoutes } from '../features/veterinary/PatientsRoutes';
import { VaccinesPage } from '../features/veterinary/VaccinesPage';
import { LoginPage, type LoginNotice } from '../features/auth/LoginPage';
import { NoAccess, NotFound, SectionPlaceholder } from '../features/sections/SectionPage';
import { Shell } from '../shared/layout/Shell';
import { Loading } from '../shared/ui';
import { homePath, navItem, sections, type SectionKey } from './navigation';

function RequireAuth() {
  const { status } = useAuth();
  const location = useLocation();
  if (status === 'restoring') return <Loading label="Проверяем сессию…" />;
  if (status === 'anonymous') {
    const state: LoginNotice = { from: location.pathname + location.search };
    return <Navigate to="/login" replace state={state} />;
  }
  return <Outlet />;
}

/** A section opens only when the active profile's menu contains it. */
function RoleSection({ section, children }: { section: SectionKey; children?: React.ReactNode }) {
  const { credentials } = useAuth();
  if (!credentials) return null;
  const role = credentials.profile.role;
  if (!navItem(role, section)) {
    // «/» is the dashboard of owner/staff/leader; other roles start elsewhere.
    return section === 'home' ? <Navigate to={homePath(role)} replace /> : <NoAccess />;
  }
  return <>{children ?? <SectionPlaceholder section={section} />}</>;
}

// Sections built so far; the rest show a placeholder with the step of the plan.
const screens: Partial<Record<SectionKey, React.ReactNode>> = {
  dogs: <DogsRoutes />,
  members: <MembersRoutes />,
  card: <OwnCardPage />,
  patients: <PatientsRoutes />,
  vaccines: <VaccinesPage />,
};

const sectionRoutes = (Object.keys(sections) as SectionKey[]).map((key) => ({
  path: sections[key].path === '/' ? undefined : `${sections[key].path}/*`,
  index: sections[key].path === '/',
  element: <RoleSection section={key}>{screens[key]}</RoleSection>,
}));

export const routes: RouteObject[] = [
  { path: '/login', element: <LoginPage /> },
  {
    element: <RequireAuth />,
    children: [
      {
        element: <Shell />,
        children: [...sectionRoutes, { path: '/account', element: <AccountPage /> }, { path: '*', element: <NotFound /> }],
      },
    ],
  },
];
