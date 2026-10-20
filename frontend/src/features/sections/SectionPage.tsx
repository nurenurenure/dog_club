import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { navItem, roleLabels, sections, type SectionKey } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { Alert, Bone, Card, EmptyState, PageHeader } from '../../shared/ui';

/** Temporary screen for sections whose step of the frontend plan is still ahead. */
export function SectionPlaceholder({ section }: { section: SectionKey }) {
  const { profile } = useSession();
  const item = navItem(profile.role, section);
  return (
    <>
      <PageHeader eyebrow={roleLabels[profile.role]} title={item?.label ?? 'Раздел'} />
      <Card>
        <EmptyState title="Раздел появится в следующих шагах" illustration={<Bone />}>
          <p>Экран строится на шаге {sections[section].step} плана фронтенда. Меню и права профиля уже работают.</p>
        </EmptyState>
      </Card>
    </>
  );
}

/** Shown when the active profile has no such section: profiles never combine rights. */
export function NoAccess() {
  const { profile } = useSession();
  return (
    <>
      <PageHeader title="Нет доступа" />
      <Alert tone="info" title={`Раздел недоступен профилю «${roleLabels[profile.role]}»`}>
        Права профилей не складываются: в каждой сессии работает один профиль. Переключите профиль в меню аккаунта или <Link to="/">вернитесь на главную</Link>.
      </Alert>
    </>
  );
}

export function NotFound() {
  return (
    <>
      <PageHeader title="Страница не найдена" />
      <Card>
        <EmptyState title="Такой страницы нет" illustration={<Bone />}>
          <Link to="/">На главную</Link>
        </EmptyState>
      </Card>
    </>
  );
}

/** Registration and editing screens: leader reads the same data without changes. */
export function StaffOnly({ children }: { children: ReactNode }) {
  const { profile } = useSession();
  if (profile.role === 'staff') return <>{children}</>;
  return (
    <>
      <PageHeader title="Только просмотр" />
      <Alert tone="info" title={`Профиль «${roleLabels[profile.role]}» не меняет записи`}>
        Изменения вносит сотрудник клуба. <Link to="..">Вернуться</Link>
      </Alert>
    </>
  );
}
