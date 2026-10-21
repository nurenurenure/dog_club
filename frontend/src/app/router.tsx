import { Navigate, Outlet, useLocation, type RouteObject } from 'react-router';
import { useAuth } from '../auth/AuthProvider';
import { AccountPage } from '../features/account/AccountPage';
import { InstructorsPage } from '../features/admin/InstructorsPage';
import { SystemPage } from '../features/admin/SystemPage';
import { UsersRoutes } from '../features/admin/UsersRoutes';
import { BreedingRoutes } from '../features/breeding/BreedingRoutes';
import { DocumentsRoutes } from '../features/documents/DocumentsRoutes';
import { DogsRoutes } from '../features/dogs/DogsRoutes';
import { EventsRoutes } from '../features/events/EventsRoutes';
import { HomePage } from '../features/home/HomePage';
import { OwnCardPage } from '../features/members/MemberPage';
import { MembersRoutes } from '../features/members/MembersRoutes';
import { ReportsPage } from '../features/reports/ReportsPage';
import { TrainingRoutes } from '../features/training/TrainingRoutes';
import { PatientsRoutes } from '../features/veterinary/PatientsRoutes';
import { VaccinesPage } from '../features/veterinary/VaccinesPage';
import { LoginPage, type LoginNotice } from '../features/auth/LoginPage';
import { NoAccess, NotFound } from '../features/sections/SectionPage';
import { Shell } from '../shared/layout/Shell';
import { Loading } from '../shared/ui';
import { homePath, navItem, sections, type SectionKey } from './navigation';

function RequireAuth() {
  const { status, signedOut } = useAuth();
  const location = useLocation();
  if (status === 'restoring') return <Loading label="Проверяем сессию…" />;
  if (status === 'anonymous') {
    // An expired session returns to its page after login; a deliberate sign-out does not.
    const state: LoginNotice = signedOut ? {} : { from: location.pathname + location.search };
    return <Navigate to="/login" replace state={state} />;
  }
  return <Outlet />;
}

/** A section opens only when the active profile's menu contains it. */
function RoleSection({ section, children }: { section: SectionKey; children: React.ReactNode }) {
  const { credentials } = useAuth();
  if (!credentials) return null;
  const role = credentials.profile.role;
  if (!navItem(role, section)) {
    // «/» is the dashboard of owner/staff/leader; other roles start elsewhere.
    return section === 'home' ? <Navigate to={homePath(role)} replace /> : <NoAccess />;
  }
  return <>{children}</>;
}

// Every section of the menu has its screen: the full record makes a missing one a type error.
const screens: Record<SectionKey, React.ReactNode> = {
  home: <HomePage />,
  dogs: <DogsRoutes />,
  members: <MembersRoutes />,
  card: <OwnCardPage />,
  patients: <PatientsRoutes />,
  vaccines: <VaccinesPage />,
  breeding: <BreedingRoutes />,
  events: <EventsRoutes />,
  training: <TrainingRoutes />,
  documents: <DocumentsRoutes />,
  reports: <ReportsPage />,
  users: <UsersRoutes />,
  instructors: <InstructorsPage />,
  system: <SystemPage />,
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
