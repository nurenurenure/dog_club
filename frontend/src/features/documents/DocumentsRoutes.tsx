import type { ReactNode } from 'react';
import { Link, Route, Routes } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { Alert, PageHeader } from '../../shared/ui';
import { NotFound } from '../sections/SectionPage';
import { DocumentCreatePage, DocumentEditPage } from './DocumentFormPage';
import { DocumentPage } from './DocumentPage';
import { DocumentsPage } from './DocumentsPage';

/** Staff keeps general and breeding records, the veterinarian medical ones; others read. */
function WritersOnly({ children }: { children: ReactNode }) {
  const { profile } = useSession();
  if (profile.role === 'staff' || profile.role === 'veterinarian') return <>{children}</>;
  return (
    <>
      <PageHeader title="Только просмотр" />
      <Alert tone="info" title={`Профиль «${roleLabels[profile.role]}» не меняет документы`}>
        Документы оформляют сотрудник клуба и ветеринар. <Link to="/documents">К документам</Link>
      </Alert>
    </>
  );
}

export function DocumentsRoutes() {
  return (
    <Routes>
      <Route index element={<DocumentsPage />} />
      <Route
        path="new"
        element={
          <WritersOnly>
            <DocumentCreatePage />
          </WritersOnly>
        }
      />
      <Route path=":documentID" element={<DocumentPage />} />
      <Route
        path=":documentID/edit"
        element={
          <WritersOnly>
            <DocumentEditPage />
          </WritersOnly>
        }
      />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
