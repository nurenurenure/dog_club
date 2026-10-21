import { useState } from 'react';
import { Link } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatDate, todayUTC } from '../../shared/format';
import { Ball, ButtonLink, Card, Chip, EmptyState, Fact, Facts, Loading, PageHeader, QueryError } from '../../shared/ui';
import { useRequests } from '../breeding/breedingApi';
import { DogName } from '../breeding/parts';
import { DocumentStatus } from '../documents/parts';
import { useDocuments } from '../documents/documentsApi';
import { DogStatusChip } from '../dogs/DogStatusChip';
import { useDogs, type DogSummary } from '../dogs/dogsApi';
import { useEvents, type Event } from '../events/eventsApi';
import { EventStatusChip } from '../events/parts';
import { formatPeriod } from '../events/zonedTime';
import { useOwnMember } from '../members/membersApi';
import { useReport } from '../reports/ReportsPage';
import { decisions } from '../veterinary/labels';
import { useMedicalStatus } from '../veterinary/vetApi';
import styles from './home.module.css';

/** «Главная» of an owner and «Обзор» of staff and leader; other roles start in their sections. */
export function HomePage() {
  const { profile } = useSession();
  return profile.role === 'owner' ? <OwnerHome /> : <OfficeHome />;
}

// The calendar filters by overlap: events still running or ahead of now, nearest first.
// The moment is fixed once per visit: a fresh one on every render would change the query key.
function useUpcoming() {
  const [from] = useState(() => new Date().toISOString());
  const events = useEvents({ from });
  const rows = (events.data?.pages.flatMap((page) => page.records) ?? [])
    .filter((event) => event.status !== 'cancelled' && event.status !== 'draft' && event.starts_at)
    .sort((a, b) => Date.parse(a.starts_at!) - Date.parse(b.starts_at!))
    .slice(0, 5);
  return { events, rows };
}

function UpcomingEvents() {
  const { events, rows } = useUpcoming();
  return (
    <Card heading="Ближайшие мероприятия">
      {events.isPending ? (
        <Loading />
      ) : events.isError ? (
        <QueryError error={events.error} onRetry={() => void events.refetch()} />
      ) : rows.length === 0 ? (
        <EmptyState title="Ближайших мероприятий нет" />
      ) : (
        <ul className={styles.list}>
          {rows.map((event) => (
            <EventRow key={event.id} event={event} />
          ))}
        </ul>
      )}
      <Link to="/events">Весь календарь</Link>
    </Card>
  );
}

function EventRow({ event }: { event: Event }) {
  return (
    <li className={styles.row}>
      <span className={styles.rowBody}>
        <Link to={`/events/${event.id}`}>{event.name}</Link>
        <span className={styles.meta}>
          {event.type.name} · {formatPeriod(event.starts_at, event.ends_at, event.timezone)}
        </span>
      </span>
      <EventStatusChip status={event.status} />
    </li>
  );
}

function OwnerHome() {
  const member = useOwnMember();
  const dogs = useDogs({});
  const documents = useDocuments({});
  // An inactive membership closes the owner's sections: say so once, at the top.
  if (member.isError) return <QueryError error={member.error} onRetry={() => void member.refetch()} />;
  const rows = dogs.data?.pages.flatMap((page) => page.dogs) ?? [];
  // The registry pages in ID order: the newest records are the last ones.
  const recent = (documents.data?.pages.flatMap((page) => page.records) ?? []).sort((a, b) => b.id - a.id).slice(0, 3);
  return (
    <>
      <PageHeader eyebrow={member.data?.full_name ?? roleLabels.owner} title="Главная" />
      <section aria-label="Мои собаки" className={styles.dogs}>
        {dogs.isPending ? (
          <Loading label="Загружаем собак…" />
        ) : dogs.isError ? (
          <QueryError error={dogs.error} onRetry={() => void dogs.refetch()} />
        ) : rows.length === 0 ? (
          <Card>
            <EmptyState title="За вами пока не записано собак" illustration={<Ball />}>
              <p>Собак ставит на учёт сотрудник клуба.</p>
            </EmptyState>
          </Card>
        ) : (
          rows.map((dog) => <DogTile key={dog.id} dog={dog} />)
        )}
      </section>
      <UpcomingEvents />
      <Card heading="Последние документы">
        {documents.isPending ? (
          <Loading />
        ) : documents.isError ? (
          <QueryError error={documents.error} onRetry={() => void documents.refetch()} />
        ) : recent.length === 0 ? (
          <EmptyState title="Документов пока нет" />
        ) : (
          <ul className={styles.list}>
            {recent.map((document) => (
              <li key={document.id} className={styles.row}>
                <span className={styles.rowBody}>
                  <Link to={`/documents/${document.id}`}>{document.type_name}</Link>
                  <span className={styles.meta}>
                    {document.number ? `№ ${document.number} · ` : ''}выдан {formatDate(document.issue_date)}
                  </span>
                </span>
                <span>
                  <DocumentStatus document={document} />
                </span>
              </li>
            ))}
          </ul>
        )}
        <Link to="/documents">Все документы</Link>
      </Card>
      <div className={styles.links}>
        <ButtonLink to="/training" variant="secondary">
          Дрессировка
        </ButtonLink>
        <ButtonLink to="/me" variant="secondary">
          Моя карточка
        </ButtonLink>
      </div>
    </>
  );
}

/** A dog with its admission to events: the owner sees at once whether a show is possible. */
function DogTile({ dog }: { dog: DogSummary }) {
  const status = useMedicalStatus(dog.id);
  const events = status.data?.events;
  return (
    <Link to={`/dogs/${dog.id}`} className={styles.tile}>
      <span className={styles.nickname}>{dog.nickname}</span>
      <span className={styles.meta}>
        {dog.breed.name} · {dog.sex.name}
      </span>
      <span className={styles.chips}>
        <DogStatusChip status={dog.status} />
        {events && (
          <Chip tone={decisions[events.decision].tone}>
            Мероприятия: {decisions[events.decision].label.toLowerCase()}
            {events.valid_until ? ` до ${formatDate(events.valid_until)}` : ''}
          </Chip>
        )}
      </span>
    </Link>
  );
}

function OfficeHome() {
  const { profile } = useSession();
  const requests = useRequests();
  const today = todayUTC();
  const report = useReport({ from: today, to: today });
  const pending = (requests.data ?? []).filter((request) => request.status === 'pending');
  const quality = report.data?.data_quality;
  const gaps = quality
    ? quality.members_without_membership_date + quality.dogs_without_registration_date + quality.performed_matings_without_date + quality.events_without_period
    : 0;

  return (
    <>
      <PageHeader eyebrow={roleLabels[profile.role]} title="Обзор" />
      {report.isError ? (
        <QueryError error={report.error} onRetry={() => void report.refetch()} />
      ) : (
        <Facts>
          <Fact label="Члены клуба" value={report.data?.current.member_cards ?? '…'} note={report.data ? `активных: ${report.data.current.active_members}` : undefined} />
          <Fact label="Собаки" value={report.data?.current.dog_cards ?? '…'} note={report.data ? `на учёте: ${report.data.current.dogs_on_register}` : undefined} />
          <Fact label="Заявки на вязку" value={requests.data ? pending.length : '…'} note="ждут решения эксперта" />
          <Fact label="Пропуски дат" value={report.data ? gaps : '…'} note={<Link to="/reports">Отчёт</Link>} />
        </Facts>
      )}
      <UpcomingEvents />
      <Card heading="Заявки, ждущие эксперта">
        {requests.isPending ? (
          <Loading />
        ) : requests.isError ? (
          <QueryError error={requests.error} onRetry={() => void requests.refetch()} />
        ) : pending.length === 0 ? (
          <EmptyState title="Ожидающих заявок нет" />
        ) : (
          <ul className={styles.list}>
            {pending.slice(0, 5).map((request) => (
              <li key={request.id} className={styles.row}>
                <span className={styles.rowBody}>
                  <Link to={`/breeding/requests/${request.id}`}>Заявка {request.id}</Link>
                  <span className={styles.meta}>
                    <DogName id={request.female_dog_id} plain /> × <DogName id={request.male_dog_id} plain /> · {formatDate(request.planned_date)}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
        <Link to="/breeding">Племенная работа</Link>
      </Card>
      {quality && gaps > 0 && (
        <Card heading="Полнота данных">
          <ul className={styles.gaps}>
            {quality.dogs_without_registration_date > 0 && <li>Собак без подтверждённой даты регистрации: {quality.dogs_without_registration_date}</li>}
            {quality.members_without_membership_date > 0 && <li>Членов клуба без даты вступления: {quality.members_without_membership_date}</li>}
            {quality.performed_matings_without_date > 0 && <li>Проведённых вязок без даты: {quality.performed_matings_without_date}</li>}
            {quality.events_without_period > 0 && <li>Мероприятий без периода: {quality.events_without_period}</li>}
          </ul>
          <p className={styles.meta}>Такие записи не попадают в отчёты за период.</p>
        </Card>
      )}
    </>
  );
}
