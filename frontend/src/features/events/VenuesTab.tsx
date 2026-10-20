import { useState } from 'react';
import { fieldErrors } from '../../api/problem';
import { useSession } from '../../auth/AuthProvider';
import { Ball, Button, Card, EmptyState, FormDialog, Loading, QueryError, ReasonDialog, Table, TextField, fieldStyles, tableStyles } from '../../shared/ui';
import { eventsApi, useVenues, useVenuesSaved, type Venue } from './eventsApi';
import { EventHistory } from './parts';

interface Draft {
  name: string;
  locality: string;
  street: string;
  building_number: string;
  room: string;
}

const emptyDraft: Draft = { name: '', locality: '', street: '', building_number: '', room: '' };

export function venueAddress(venue: Venue): string {
  return [venue.locality, venue.street, venue.building_number, venue.room].filter(Boolean).join(', ');
}

/** Venues have their own address, not tied to a person; staff edits them until facts appear. */
export function VenuesTab() {
  const { profile } = useSession();
  const staff = profile.role === 'staff';
  const venues = useVenues();
  const saved = useVenuesSaved();
  const [editing, setEditing] = useState<Venue | 'new' | null>(null);
  const [history, setHistory] = useState<number | null>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [error, setError] = useState<unknown>(null);
  const errors = fieldErrors(error);

  const open = (venue: Venue | 'new') => {
    setDraft(venue === 'new' ? emptyDraft : { name: venue.name, locality: venue.locality, street: venue.street, building_number: venue.building_number, room: venue.room });
    setError(null);
    setEditing(venue);
  };
  const run = async (action: () => Promise<unknown>) => {
    try {
      await action();
      await saved();
    } catch (failure) {
      setError(failure);
      throw failure;
    }
  };
  const body = () => ({
    name: draft.name.trim(),
    locality: draft.locality.trim(),
    street: draft.street.trim(),
    building_number: draft.building_number.trim(),
    room: draft.room.trim(),
  });
  const ready = Boolean(draft.name.trim() && draft.locality.trim());
  const fields = (
    <>
      <TextField label="Название" required maxLength={150} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} error={errors.name} />
      <TextField
        label="Населённый пункт"
        required
        maxLength={100}
        value={draft.locality}
        onChange={(event) => setDraft({ ...draft, locality: event.target.value })}
        error={errors.locality}
      />
      <TextField label="Улица" maxLength={100} value={draft.street} onChange={(event) => setDraft({ ...draft, street: event.target.value })} error={errors.street} />
      <div className={fieldStyles.row}>
        <TextField
          label="Дом"
          maxLength={20}
          value={draft.building_number}
          onChange={(event) => setDraft({ ...draft, building_number: event.target.value })}
          error={errors.building_number}
        />
        <TextField label="Помещение" maxLength={20} value={draft.room} onChange={(event) => setDraft({ ...draft, room: event.target.value })} error={errors.room} />
      </div>
    </>
  );

  if (venues.isPending) return <Loading label="Загружаем площадки…" />;
  if (venues.isError) return <QueryError error={venues.error} onRetry={() => void venues.refetch()} />;
  return (
    <>
      {staff && (
        <div>
          <Button variant="save" onClick={() => open('new')}>
            Новая площадка
          </Button>
        </div>
      )}
      {venues.data.length === 0 ? (
        <Card>
          <EmptyState title="Площадок пока нет" illustration={<Ball />} />
        </Card>
      ) : (
        <Table caption="Площадки">
          <thead>
            <tr>
              <th scope="col">Название</th>
              <th scope="col">Адрес</th>
              <th scope="col">Действия</th>
            </tr>
          </thead>
          <tbody>
            {venues.data.map((venue) => (
              <tr key={venue.id}>
                <td>{venue.name}</td>
                <td>{venueAddress(venue)}</td>
                <td>
                  <span className={tableStyles.actions}>
                    <Button variant="link" onClick={() => setHistory(history === venue.id ? null : venue.id)}>
                      {history === venue.id ? 'Скрыть историю' : 'История'}
                    </Button>
                    {staff && (
                      <Button variant="secondary" onClick={() => open(venue)}>
                        Исправить
                      </Button>
                    )}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      {history !== null && <EventHistory target={{ kind: 'venue', id: history }} />}

      <FormDialog
        open={editing === 'new'}
        title="Новая площадка"
        confirmLabel="Создать"
        ready={ready}
        onClose={() => setEditing(null)}
        onSubmit={() => run(() => eventsApi.createVenue(body()))}
      >
        {fields}
      </FormDialog>
      <ReasonDialog
        open={editing !== null && editing !== 'new'}
        title="Исправить площадку"
        description="Площадку нельзя переписать после фактов участия в её мероприятиях."
        confirmLabel="Сохранить"
        ready={ready}
        onReload={() => void venues.refetch()}
        onClose={() => setEditing(null)}
        onConfirm={(reason) => run(() => eventsApi.updateVenue((editing as Venue).id, { ...body(), version: (editing as Venue).version, reason }))}
      >
        {fields}
      </ReasonDialog>
    </>
  );
}
