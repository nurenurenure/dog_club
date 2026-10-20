import { Plus } from 'lucide-react';
import { Link, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatDate } from '../../shared/format';
import { Ball, ButtonLink, Card, Chip, EmptyState, Loading, PageHeader, QueryError, Table, Tabs, tabPanelProps, tableStyles } from '../../shared/ui';
import { useLitters, useMatings, useRequests, type BreedingRequest } from './breedingApi';
import { outcomes } from './labels';
import { DogName, RequestStatus } from './parts';
import styles from './breeding.module.css';

const tabs = [
  { value: 'requests', label: 'Заявки' },
  { value: 'matings', label: 'Вязки' },
  { value: 'litters', label: 'Помёты' },
] as const;
type Tab = (typeof tabs)[number]['value'];

/** Staff runs the process, the expert decides on requests, the leader reads. */
export function BreedingPage() {
  const { profile } = useSession();
  const expert = profile.role === 'expert';
  const [params, setParams] = useSearchParams();
  const tab: Tab = tabs.find((item) => item.value === params.get('tab'))?.value ?? 'requests';
  return (
    <>
      <PageHeader
        eyebrow={roleLabels[profile.role]}
        title={expert ? 'Племенные заявки' : 'Племенная работа'}
        actions={
          profile.role === 'staff' && (
            <ButtonLink to="/breeding/requests/new">
              <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
              Новая заявка
            </ButtonLink>
          )
        }
      />
      <Tabs label="Разделы" idPrefix="breeding" value={tab} options={tabs} onChange={(value) => setParams(value === 'requests' ? {} : { tab: value }, { replace: true })} />
      <section {...tabPanelProps('breeding', tab)} className={styles.panel}>
        {tab === 'requests' && <RequestsTable pendingFirst={expert} />}
        {tab === 'matings' && <MatingsTable />}
        {tab === 'litters' && <LittersTable />}
      </section>
    </>
  );
}

/** Owners have no breeding section: their rows are plain text. */
function useLinkable() {
  return useSession().profile.role !== 'owner';
}

const statusOrder: Record<BreedingRequest['status'], number> = { pending: 0, approved: 1, rejected: 2, cancelled: 3 };

export function RequestsTable({ dogID, pendingFirst }: { dogID?: number; pendingFirst?: boolean }) {
  const linkable = useLinkable();
  const requests = useRequests(dogID);
  if (requests.isPending) return <Loading />;
  if (requests.isError) return <QueryError error={requests.error} onRetry={() => void requests.refetch()} />;
  // The expert's work queue: pending requests on top, otherwise newest first.
  const rows = pendingFirst ? [...requests.data].sort((a, b) => statusOrder[a.status] - statusOrder[b.status]) : requests.data;
  if (rows.length === 0) {
    return (
      <Card>
        <EmptyState title="Заявок пока нет" illustration={<Ball />} />
      </Card>
    );
  }
  return (
    <Table caption="Заявки на вязку">
      <thead>
        <tr>
          <th scope="col">№</th>
          <th scope="col">Сука</th>
          <th scope="col">Кобель</th>
          <th scope="col">Планируемая дата</th>
          <th scope="col">Статус</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((request) => (
          <tr key={request.id}>
            <td>{linkable ? <Link to={`/breeding/requests/${request.id}`}>Заявка {request.id}</Link> : `Заявка ${request.id}`}</td>
            <td>
              <DogName id={request.female_dog_id} plain />
            </td>
            <td>
              <DogName id={request.male_dog_id} plain />
            </td>
            <td className={tableStyles.nowrap}>{formatDate(request.planned_date)}</td>
            <td>
              <RequestStatus status={request.status} />
            </td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

export function MatingsTable({ dogID }: { dogID?: number }) {
  const linkable = useLinkable();
  const matings = useMatings(dogID);
  if (matings.isPending) return <Loading />;
  if (matings.isError) return <QueryError error={matings.error} onRetry={() => void matings.refetch()} />;
  if (matings.data.length === 0) {
    return (
      <Card>
        <EmptyState title="Проведённых вязок пока нет" illustration={<Ball />} />
      </Card>
    );
  }
  return (
    <Table caption="Вязки">
      <thead>
        <tr>
          <th scope="col">№</th>
          <th scope="col">Сука</th>
          <th scope="col">Кобель</th>
          <th scope="col">Дата</th>
          <th scope="col">Исход</th>
        </tr>
      </thead>
      <tbody>
        {matings.data.map((mating) => (
          <tr key={mating.id}>
            <td>{linkable ? <Link to={`/breeding/matings/${mating.id}`}>Вязка {mating.id}</Link> : `Вязка ${mating.id}`}</td>
            <td>
              <DogName id={mating.female_dog_id} plain />
            </td>
            <td>
              <DogName id={mating.male_dog_id} plain />
            </td>
            <td className={tableStyles.nowrap}>{formatDate(mating.date)}</td>
            <td>
              <Chip tone={outcomes[mating.outcome].tone}>{outcomes[mating.outcome].label}</Chip>
            </td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

export function LittersTable({ dogID }: { dogID?: number }) {
  const linkable = useLinkable();
  const litters = useLitters(dogID);
  if (litters.isPending) return <Loading />;
  if (litters.isError) return <QueryError error={litters.error} onRetry={() => void litters.refetch()} />;
  if (litters.data.length === 0) {
    return (
      <Card>
        <EmptyState title="Помётов пока нет" illustration={<Ball />} />
      </Card>
    );
  }
  return (
    <Table caption="Помёты">
      <thead>
        <tr>
          <th scope="col">№</th>
          <th scope="col">Мать</th>
          <th scope="col">Отец</th>
          <th scope="col">Рождение</th>
          <th scope="col">Щенки</th>
        </tr>
      </thead>
      <tbody>
        {litters.data.map((litter) => (
          <tr key={litter.id}>
            <td>{linkable ? <Link to={`/breeding/litters/${litter.id}`}>Помёт {litter.id}</Link> : `Помёт ${litter.id}`}</td>
            <td>
              <DogName id={litter.female_dog_id} plain />
            </td>
            <td>
              <DogName id={litter.male_dog_id} plain />
            </td>
            <td className={tableStyles.nowrap}>
              {formatDate(litter.date_of_birth)}
              {litter.interval_violation && (
                <>
                  {' '}
                  <Chip tone="sun">меньше 365 дней</Chip>
                </>
              )}
            </td>
            <td>
              {litter.dog_ids.length} из {litter.total_puppies}
            </td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}
