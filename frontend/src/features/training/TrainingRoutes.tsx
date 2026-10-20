import { Route, Routes } from 'react-router';
import { NotFound, StaffOnly } from '../sections/SectionPage';
import { SessionCreatePage, SessionEditPage } from './SessionFormPage';
import { SessionPage } from './SessionPage';
import { TrainingPage } from './TrainingPage';

export function TrainingRoutes() {
  return (
    <Routes>
      <Route index element={<TrainingPage />} />
      <Route
        path="new"
        element={
          <StaffOnly>
            <SessionCreatePage />
          </StaffOnly>
        }
      />
      <Route path=":dogID/:sessionID" element={<SessionPage />} />
      <Route
        path=":dogID/:sessionID/edit"
        element={
          <StaffOnly>
            <SessionEditPage />
          </StaffOnly>
        }
      />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
