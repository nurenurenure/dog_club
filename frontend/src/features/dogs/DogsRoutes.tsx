import { Route, Routes } from 'react-router';
import { NotFound, StaffOnly } from '../sections/SectionPage';
import { DogCreatePage, DogEditPage } from './DogFormPage';
import { DogPage } from './DogPage';
import { DogsListPage } from './DogsListPage';

export function DogsRoutes() {
  return (
    <Routes>
      <Route index element={<DogsListPage />} />
      <Route
        path="new"
        element={
          <StaffOnly>
            <DogCreatePage />
          </StaffOnly>
        }
      />
      <Route path=":dogID" element={<DogPage />} />
      <Route
        path=":dogID/edit"
        element={
          <StaffOnly>
            <DogEditPage />
          </StaffOnly>
        }
      />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
