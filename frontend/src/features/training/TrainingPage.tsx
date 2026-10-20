import { Plus } from 'lucide-react';
import { Link, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import {
  Alert,
  Ball,
  Button,
  ButtonLink,
  Card,
  Chip,
  EmptyState,
  Loading,
  PageHeader,
  QueryError,
  SelectField,
  Table,
  TextField,
  Toolbar,
  tableStyles,
  toOptions,
} from '../../shared/ui';
import { DogPicker } from '../dogs/DogPicker';
import { useInstructors, useOwnGrant, useTrainingSessions, useTrainingTypes, type TrainingFilter } from './trainingApi';
import { DogLabel, MedicalChip, sessionTime } from './parts';
import styles from './training.module.css';

/** A journal of sessions already held; owners see their current dogs only. */
export function TrainingPage() {
  const { profile } = useSession();
  const owner = profile.role === 'owner';
  const office = !owner;
  const [params, setParams] = useSearchParams();
  const types = useTrainingTypes();
  const instructors = useInstructors(office);
  const grant = useOwnGrant();
  const dogID = Number(params.get('dog')) || undefined;
  const from = params.get('from') ?? '';
  const to = params.get('to') ?? '';
  const filter: TrainingFilter = {
    dog_id: dogID,
    type_id: Number(params.get('type')) || undefined,
    instructor_profile_id: params.get('instructor') ?? undefined,
    // Bounds of the start time, inclusive; whole UTC days are enough for a journal search.
    from: from ? `${from}T00:00:00Z` : undefined,
    to: to ? `${to}T23:59:59Z` : undefined,
  };

  const setParam = (name: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(name, value);
    else next.delete(name);
    setParams(next, { replace: true });
  };

  return (
    <>
      <PageHeader
        eyebrow={roleLabels[profile.role]}
        title={owner ? 'Дрессировка моих собак' : 'Журнал дрессировки'}
        actions={
          grant.typeIDs.length > 0 && (
            <ButtonLink to={dogID ? `/training/new?dog=${dogID}` : '/training/new'}>
              <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
              Внести занятие
            </ButtonLink>
          )
        }
      />
      {profile.role === 'staff' && !grant.pending && grant.typeIDs.length === 0 && (
        <Alert tone="info">Вносить занятия может сотрудник с допуском инструктора: его выдаёт администратор клуба.</Alert>
      )}
      <Toolbar>
        {office &&
          (dogID ? (
            <div className={styles.picked}>
              <span>
                Собака: <b>{<DogLabel id={dogID} link={false} />}</b>
              </span>
              <Button variant="link" onClick={() => setParam('dog', '')}>
                Все собаки
              </Button>
            </div>
          ) : (
            <DogPicker label="Собака" onPick={(dog) => setParam('dog', String(dog.id))} />
          ))}
        <SelectField
          label="Вид"
          placeholder="Все виды"
          options={toOptions(types.data ?? [])}
          value={params.get('type') ?? ''}
          onChange={(event) => setParam('type', event.target.value)}
        />
        {office && (
          <SelectField
            label="Инструктор"
            placeholder="Все инструкторы"
            options={(instructors.data ?? []).map((item) => ({ value: item.profile_id, label: item.name }))}
            value={params.get('instructor') ?? ''}
            onChange={(event) => setParam('instructor', event.target.value)}
          />
        )}
        <TextField label="С даты" type="date" value={from} onChange={(event) => setParam('from', event.target.value)} />
        <TextField label="По дату" type="date" min={from || undefined} value={to} onChange={(event) => setParam('to', event.target.value)} />
      </Toolbar>
      <SessionsTable filter={filter} filtered={params.toString() !== ''} />
    </>
  );
}

/** Sessions newest last as the server pages them; voided ones stay crossed out. */
export function SessionsTable({ filter, filtered, hideDog }: { filter: TrainingFilter; filtered?: boolean; hideDog?: boolean }) {
  const sessions = useTrainingSessions(filter);
  const rows = sessions.data?.pages.flatMap((page) => page.records) ?? [];
  if (sessions.isPending) return <Loading label="Загружаем занятия…" />;
  if (sessions.isError) return <QueryError error={sessions.error} onRetry={() => void sessions.refetch()} />;
  if (rows.length === 0) {
    return (
      <Card>
        <EmptyState title={filtered ? 'Ничего не нашли' : 'Занятий пока нет'} illustration={<Ball />}>
          {filtered && <p>Измените фильтры.</p>}
        </EmptyState>
      </Card>
    );
  }
  return (
    <>
      <Table caption="Занятия">
        <thead>
          <tr>
            <th scope="col">Время</th>
            {!hideDog && <th scope="col">Собака</th>}
            <th scope="col">Вид</th>
            <th scope="col">Инструктор</th>
            <th scope="col">Допуск</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((session) => (
            <tr key={`${session.dog_id}-${session.id}`} className={session.is_void ? styles.void : undefined}>
              <td className={tableStyles.nowrap}>
                <Link to={`/training/${session.dog_id}/${session.id}`}>{sessionTime(session)}</Link>
                {session.is_void && (
                  <div>
                    <Chip tone="dashed" struck>
                      Ошибочное
                    </Chip>
                  </div>
                )}
              </td>
              {!hideDog && (
                <td>
                  <DogLabel id={session.dog_id} />
                </td>
              )}
              <td>{session.type.name}</td>
              <td>{session.instructor_name || '—'}</td>
              <td>{session.legacy ? <Chip tone="dashed">Старая запись</Chip> : <MedicalChip check={session.current_medical ?? session.medical_snapshot} />}</td>
            </tr>
          ))}
        </tbody>
      </Table>
      {sessions.hasNextPage && (
        <Button variant="secondary" className={tableStyles.more} busy={sessions.isFetchingNextPage} onClick={() => void sessions.fetchNextPage()}>
          Следующие 50
        </Button>
      )}
    </>
  );
}
