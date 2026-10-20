import { Route, Routes } from 'react-router';
import { NotFound, StaffOnly } from '../sections/SectionPage';
import { BreedingPage } from './BreedingPage';
import { LitterPage } from './LitterPage';
import { MateCandidatesPage } from './MateCandidatesPage';
import { MatingPage } from './MatingPage';
import { RequestCreatePage, RequestEditPage } from './RequestFormPage';
import { RequestPage } from './RequestPage';

export function BreedingRoutes() {
  return (
    <Routes>
      <Route index element={<BreedingPage />} />
      <Route
        path="requests/new"
        element={
          <StaffOnly>
            <RequestCreatePage />
          </StaffOnly>
        }
      />
      <Route path="requests/:requestID" element={<RequestPage />} />
      <Route
        path="requests/:requestID/edit"
        element={
          <StaffOnly>
            <RequestEditPage />
          </StaffOnly>
        }
      />
      <Route path="matings/:matingID" element={<MatingPage />} />
      <Route path="litters/:litterID" element={<LitterPage />} />
      <Route path="candidates/:dogID" element={<MateCandidatesPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
