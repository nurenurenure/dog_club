import { Route, Routes } from 'react-router';
import { NotFound, StaffOnly } from '../sections/SectionPage';
import { EventCreatePage, EventEditPage } from './EventFormPage';
import { EventPage } from './EventPage';
import { EventsPage } from './EventsPage';
import { RegistrationPage } from './RegistrationPage';

export function EventsRoutes() {
  return (
    <Routes>
      <Route index element={<EventsPage />} />
      <Route
        path="new"
        element={
          <StaffOnly>
            <EventCreatePage />
          </StaffOnly>
        }
      />
      <Route path=":eventID" element={<EventPage />} />
      <Route
        path=":eventID/edit"
        element={
          <StaffOnly>
            <EventEditPage />
          </StaffOnly>
        }
      />
      <Route path=":eventID/registrations/:registrationID" element={<RegistrationPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
