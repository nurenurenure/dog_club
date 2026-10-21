import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatAge, formatDate } from '../../shared/format';
import { Ball, Button, Card, Chip, EmptyState, Loading, PageHeader, QueryError, Table, Tabs, TextField, tabPanelProps, tableStyles } from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { DogPhoto } from '../dogs/DogPhoto';
import { HealthTab, InactiveNote } from './HealthTab';
import { activeByName } from './labels';
import { VetHistory } from './VetHistory';
import { usePatient, usePatients } from './vetApi';
import styles from './vet.module.css';

/** Patient search: the veterinarian sees animals, never their owners. */
export function PatientsPage() {
  const { profile } = useSession();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get('q') ?? '');
  const q = useDebounced(query.trim());
  const patients = usePatients(q);
  const rows = patients.data?.pages.flatMap((page) => page.dogs) ?? [];
  return (
    <>
      <PageHeader eyebrow={roleLabels[profile.role]} title="Пациенты" />
      <div className={styles.toolbar}>
        <TextField
          label="Поиск"
          type="search"
          placeholder="Кличка, идентификатор или номер родословной"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setParams(event.target.value.trim() ? { q: event.target.value.trim() } : {}, { replace: true });
          }}
        />
      </div>
      {patients.isPending ? (
        <Loading label="Ищем пациентов…" />
      ) : patients.isError ? (
        <QueryError error={patients.error} onRetry={() => void patients.refetch()} />
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState title={q ? 'Никого не нашли' : 'Пациентов пока нет'} illustration={<Ball />} />
        </Card>
      ) : (
        <>
          <Table caption="Пациенты">
            <thead>
              <tr>
                <th scope="col">Кличка</th>
                <th scope="col">Порода</th>
                <th scope="col">Пол</th>
                <th scope="col">Рождение</th>
                <th scope="col">Идентификатор</th>
                <th scope="col">Статус</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((dog) => (
                <tr key={dog.id}>
                  <td>
                    <Link to={`/patients/${dog.id}`}>{dog.nickname}</Link>
                  </td>
                  <td>{dog.breed.name}</td>
                  <td>{dog.sex.name}</td>
                  <td className={tableStyles.nowrap}>{formatDate(dog.date_of_birth)}</td>
                  <td className={tableStyles.nowrap}>{dog.identifier.number}</td>
                  <td>{activeByName(dog.status) ? <Chip tone="lime">{dog.status.name}</Chip> : <Chip struck>{dog.status.name}</Chip>}</td>
                </tr>
              ))}
            </tbody>
          </Table>
          {patients.hasNextPage && (
            <Button variant="secondary" className={tableStyles.more} busy={patients.isFetchingNextPage} onClick={() => void patients.fetchNextPage()}>
              Следующие 50
            </Button>
          )}
        </>
      )}
    </>
  );
}

const tabs = [
  { value: 'health', label: 'Здоровье' },
  { value: 'history', label: 'История' },
] as const;

export function PatientPage() {
  const id = Number(useParams().dogID);
  const patient = usePatient(id);
  const [params, setParams] = useSearchParams();
  const tab = params.get('tab') === 'history' ? 'history' : 'health';
  if (patient.isPending) return <Loading label="Загружаем пациента…" />;
  if (patient.isError) return <QueryError error={patient.error} onRetry={() => void patient.refetch()} />;
  const dog = patient.data;
  if (!dog) {
    return (
      <Card>
        <EmptyState title="Пациент не найден" illustration={<Ball />}>
          <Link to="/patients">К списку пациентов</Link>
        </EmptyState>
      </Card>
    );
  }
  const active = activeByName(dog.status);
  const age = formatAge(dog.date_of_birth);
  return (
    <>
      <nav aria-label="Навигационная цепочка" className={styles.sub}>
        <Link to="/patients">Пациенты</Link> / {dog.nickname}
      </nav>
      <PageHeader eyebrow={`${dog.breed.name} · ${dog.sex.name}`} title={dog.nickname} />
      <DogPhoto dogID={dog.id} nickname={dog.nickname} iconSize={48} className={styles.patientPhoto} />
      <p className={styles.patientMeta}>
        {active ? <Chip tone="lime">{dog.status.name}</Chip> : <Chip struck>{dog.status.name}</Chip>}
        <span>
          Род. {formatDate(dog.date_of_birth)}
          {age && ` · ${age}`}
        </span>
        <span>
          {dog.identifier.type.name} <b>{dog.identifier.number}</b>
        </span>
      </p>
      {!active && <InactiveNote />}
      <Tabs label="Разделы пациента" idPrefix="patient" value={tab} options={tabs} onChange={(value) => setParams(value === 'health' ? {} : { tab: value }, { replace: true })} />
      <section {...tabPanelProps('patient', tab)} className={styles.panel}>
        {tab === 'health' ? <HealthTab dogID={dog.id} birth={dog.date_of_birth} active={active} /> : <VetHistory dogID={dog.id} />}
      </section>
    </>
  );
}
