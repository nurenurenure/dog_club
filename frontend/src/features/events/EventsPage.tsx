import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatDate } from '../../shared/format';
import {
  Ball,
  Button,
  ButtonLink,
  Card,
  EmptyState,
  Loading,
  PageHeader,
  QueryError,
  SelectField,
  Tabs,
  TextField,
  Toolbar,
  tabPanelProps,
  tableStyles,
  toOptions,
} from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { useEventDictionaries, useEvents, type Event, type EventFilter, type EventStatus } from './eventsApi';
import { statuses } from './labels';
import { EventStatusChip } from './parts';
import { VenuesTab } from './VenuesTab';
import { formatPeriod, toZonedInput } from './zonedTime';
import styles from './events.module.css';

const tabs = [
  { value: 'calendar', label: 'Календарь' },
  { value: 'venues', label: 'Площадки' },
] as const;
type Tab = (typeof tabs)[number]['value'];

const statusOptions = (Object.keys(statuses) as EventStatus[]).map((value) => ({ value, label: statuses[value].label }));

/** Staff and leader see all events, the expert assigned ones, the owner all but drafts. */
export function EventsPage() {
  const { profile } = useSession();
  const office = profile.role === 'staff' || profile.role === 'leader';
  const [params, setParams] = useSearchParams();
  const tab: Tab = office && params.get('tab') === 'venues' ? 'venues' : 'calendar';
  return (
    <>
      <PageHeader
        eyebrow={roleLabels[profile.role]}
        title={profile.role === 'expert' ? 'Мои мероприятия' : 'Мероприятия'}
        actions={
          profile.role === 'staff' &&
          tab === 'calendar' && (
            <ButtonLink to="/events/new">
              <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
              Новое мероприятие
            </ButtonLink>
          )
        }
      />
      {office && (
        <Tabs label="Разделы" idPrefix="events" value={tab} options={tabs} onChange={(value) => setParams(value === 'calendar' ? {} : { tab: value }, { replace: true })} />
      )}
      <section {...tabPanelProps('events', tab)} className={styles.panel}>
        {tab === 'calendar' ? <Calendar /> : <VenuesTab />}
      </section>
    </>
  );
}

function Calendar() {
  const { profile } = useSession();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get('q') ?? '');
  const q = useDebounced(query.trim());
  const from = params.get('from') ?? '';
  const to = params.get('to') ?? '';
  const filter: EventFilter = {
    q,
    status: (params.get('status') as EventStatus | null) ?? undefined,
    type_id: Number(params.get('type')) || undefined,
    // Dates filter by overlap of periods; whole UTC days are close enough for a search.
    from: from ? `${from}T00:00:00Z` : undefined,
    to: to ? `${to}T23:59:59Z` : undefined,
  };
  const dictionaries = useEventDictionaries();
  const events = useEvents(filter);
  const rows = events.data?.pages.flatMap((page) => page.records) ?? [];
  const filtered = [...params.keys()].some((key) => key !== 'tab');

  const setParam = (name: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(name, value);
    else next.delete(name);
    setParams(next, { replace: true });
  };

  return (
    <>
      <Toolbar>
        <TextField
          label="Поиск"
          type="search"
          placeholder="Название мероприятия"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setParam('q', event.target.value.trim());
          }}
        />
        <SelectField
          label="Тип"
          placeholder="Все типы"
          options={toOptions(dictionaries.data?.types ?? [])}
          value={params.get('type') ?? ''}
          onChange={(event) => setParam('type', event.target.value)}
        />
        <SelectField
          label="Состояние"
          placeholder={profile.role === 'owner' ? 'Все, кроме черновиков' : 'Все состояния'}
          options={profile.role === 'owner' ? statusOptions.filter((option) => option.value !== 'draft') : statusOptions}
          value={params.get('status') ?? ''}
          onChange={(event) => setParam('status', event.target.value)}
        />
        <TextField label="С даты" type="date" value={from} onChange={(event) => setParam('from', event.target.value)} />
        <TextField label="По дату" type="date" min={from || undefined} value={to} onChange={(event) => setParam('to', event.target.value)} />
      </Toolbar>
      {events.isPending ? (
        <Loading label="Загружаем мероприятия…" />
      ) : events.isError ? (
        <QueryError error={events.error} onRetry={() => void events.refetch()} />
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState title={filtered ? 'Ничего не нашли' : profile.role === 'expert' ? 'Вас пока не назначили на мероприятия' : 'Мероприятий пока нет'} illustration={<Ball />}>
            {filtered && <p>Измените поиск или фильтры.</p>}
          </EmptyState>
        </Card>
      ) : (
        <>
          <ul className={styles.calendar} aria-label="Мероприятия">
            {rows.map((event) => (
              <CalendarEntry key={event.id} event={event} />
            ))}
          </ul>
          {events.hasNextPage && (
            <Button variant="secondary" className={tableStyles.more} busy={events.isFetchingNextPage} onClick={() => void events.fetchNextPage()}>
              Следующие 50
            </Button>
          )}
        </>
      )}
    </>
  );
}

const months = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];

function CalendarEntry({ event }: { event: Event }) {
  // The tile shows the local start date; old events keep their text date without a zone.
  const local = event.starts_at ? toZonedInput(event.starts_at, event.timezone || 'UTC') : (event.legacy_starts_at ?? '');
  const [year, month, day] = local.slice(0, 10).split('-').map(Number);
  const known = Boolean(year && month && day);
  const venue = event.venue ? `${event.venue.name}, ${event.venue.locality}` : 'Площадка не указана';
  const when = event.starts_at
    ? formatPeriod(event.starts_at, event.ends_at, event.timezone)
    : event.legacy_starts_at
      ? `${formatDate(event.legacy_starts_at)} · время без пояса`
      : 'Дата не назначена';
  return (
    <li className={styles.entry}>
      <span className={known ? styles.date : `${styles.date} ${styles.dateUnknown}`} aria-hidden="true">
        {known ? (
          <>
            <span className={styles.day}>{day}</span>
            <span className={styles.month}>
              {months[month! - 1]} {year}
            </span>
          </>
        ) : (
          <span className={styles.day}>?</span>
        )}
      </span>
      <span className={styles.entryBody}>
        <Link to={`/events/${event.id}`} className={styles.entryName}>
          {event.name}
        </Link>
        <span className={styles.entryMeta}>
          {event.type.name} · {when}
        </span>
        <span className={styles.entryMeta}>{venue}</span>
      </span>
      <EventStatusChip status={event.status} />
    </li>
  );
}
