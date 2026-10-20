import { Route, Routes } from 'react-router';
import { NotFound } from '../sections/SectionPage';
import { PatientPage, PatientsPage } from './PatientsPage';

export function PatientsRoutes() {
  return (
    <Routes>
      <Route index element={<PatientsPage />} />
      <Route path=":dogID" element={<PatientPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
