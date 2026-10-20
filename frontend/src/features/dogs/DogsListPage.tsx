import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatDate } from '../../shared/format';
import { Ball, Button, ButtonLink, Card, EmptyState, Loading, PageHeader, QueryError, SelectField, Table, TextField, Toolbar, tableStyles, toOptions } from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { DogStatusChip } from './DogStatusChip';
import { useDogDictionaries, useDogs, type DogFilter } from './dogsApi';

const numberParam = (value: string | null) => (value ? Number(value) || undefined : undefined);

/** Club catalogue for staff and leader; the owner sees only their own dogs. */
export function DogsListPage() {
  const { profile } = useSession();
  const owner = profile.role === 'owner';
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get('q') ?? '');
  const q = useDebounced(query.trim());
  const filter: DogFilter = { q, breed_id: numberParam(params.get('breed')), status_id: numberParam(params.get('status')), sex_id: numberParam(params.get('sex')) };
  const dictionaries = useDogDictionaries();
  const dogs = useDogs(filter);
  const rows = dogs.data?.pages.flatMap((page) => page.dogs) ?? [];
  const filtered = params.toString() !== '';

  const setParam = (name: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(name, value);
    else next.delete(name);
    setParams(next, { replace: true });
  };
  const search = (value: string) => {
    setQuery(value);
    setParam('q', value.trim());
  };

  return (
    <>
      <PageHeader
        eyebrow={roleLabels[profile.role]}
        title={owner ? 'Мои собаки' : 'Собаки'}
        actions={
          profile.role === 'staff' && (
            <ButtonLink to="/dogs/new">
              <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
              Зарегистрировать собаку
            </ButtonLink>
          )
        }
      />
      <Toolbar>
        <TextField label="Поиск" type="search" placeholder="Кличка, идентификатор или номер родословной" value={query} onChange={(event) => search(event.target.value)} />
        {!owner && (
          <>
            <SelectField
              label="Порода"
              placeholder="Все породы"
              options={toOptions(dictionaries.data?.breeds ?? [])}
              value={params.get('breed') ?? ''}
              onChange={(event) => setParam('breed', event.target.value)}
            />
            <SelectField
              label="Пол"
              placeholder="Любой"
              options={toOptions(dictionaries.data?.sexes ?? [])}
              value={params.get('sex') ?? ''}
              onChange={(event) => setParam('sex', event.target.value)}
            />
            <SelectField
              label="Статус"
              placeholder="Все статусы"
              options={toOptions(dictionaries.data?.statuses ?? [])}
              value={params.get('status') ?? ''}
              onChange={(event) => setParam('status', event.target.value)}
            />
          </>
        )}
      </Toolbar>
      {dogs.isPending ? (
        <Loading label="Загружаем собак…" />
      ) : dogs.isError ? (
        <QueryError error={dogs.error} onRetry={() => void dogs.refetch()} />
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState title={filtered ? 'Никого не нашли' : owner ? 'За вами пока не записано собак' : 'В клубе пока нет собак'} illustration={<Ball />}>
            {filtered && <p>Измените поиск или фильтры.</p>}
          </EmptyState>
        </Card>
      ) : (
        <>
          <Table caption={owner ? 'Мои собаки' : 'Собаки клуба'}>
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
                    <Link to={`/dogs/${dog.id}`}>{dog.nickname}</Link>
                  </td>
                  <td>{dog.breed.name}</td>
                  <td>{dog.sex.name}</td>
                  <td className={tableStyles.nowrap}>{formatDate(dog.date_of_birth)}</td>
                  <td className={tableStyles.nowrap}>
                    <span className={tableStyles.muted}>{dog.identifier.type.name}</span> {dog.identifier.number}
                  </td>
                  <td>
                    <DogStatusChip status={dog.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          {dogs.hasNextPage && (
            <Button variant="secondary" className={tableStyles.more} busy={dogs.isFetchingNextPage} onClick={() => void dogs.fetchNextPage()}>
              Следующие 50
            </Button>
          )}
        </>
      )}
    </>
  );
}
