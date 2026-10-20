import { Route, Routes } from 'react-router';
import { NotFound, StaffOnly } from '../sections/SectionPage';
import { MemberCreatePage, MemberEditPage } from './MemberFormPage';
import { MemberPage } from './MemberPage';
import { MembersListPage } from './MembersListPage';

export function MembersRoutes() {
  return (
    <Routes>
      <Route index element={<MembersListPage />} />
      <Route
        path="new"
        element={
          <StaffOnly>
            <MemberCreatePage />
          </StaffOnly>
        }
      />
      <Route path=":memberID" element={<MemberPage />} />
      <Route
        path=":memberID/edit"
        element={
          <StaffOnly>
            <MemberEditPage />
          </StaffOnly>
        }
      />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
