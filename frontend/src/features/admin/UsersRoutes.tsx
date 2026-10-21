import { Route, Routes } from 'react-router';
import { NotFound } from '../sections/SectionPage';
import { UserPage } from './UserPage';
import { UsersPage } from './UsersPage';

export function UsersRoutes() {
  return (
    <Routes>
      <Route index element={<UsersPage />} />
      <Route path=":userID" element={<UserPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
