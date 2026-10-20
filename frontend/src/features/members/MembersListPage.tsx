import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatDate } from '../../shared/format';
import { Bone, Button, ButtonLink, Card, EmptyState, Loading, PageHeader, QueryError, SelectField, Table, TextField, Toolbar, tableStyles, toOptions } from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { MembershipChip } from './MembershipChip';
import { useMemberDictionaries, useMembers } from './membersApi';

export function MembersListPage() {
  const { profile } = useSession();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get('q') ?? '');
  const q = useDebounced(query.trim());
  const status = Number(params.get('status')) || undefined;
  const dictionaries = useMemberDictionaries();
  const members = useMembers({ q, membership_status_id: status });
  const rows = members.data?.pages.flatMap((page) => page.members) ?? [];
  const filtered = params.toString() !== '';

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
        title="Члены клуба"
        actions={
          profile.role === 'staff' && (
            <ButtonLink to="/members/new">
              <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
              Новый член клуба
            </ButtonLink>
          )
        }
      />
      <Toolbar>
        <TextField
          label="Поиск"
          type="search"
          placeholder="ФИО"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setParam('q', event.target.value.trim());
          }}
        />
        <SelectField
          label="Членство"
          placeholder="Все статусы"
          options={toOptions(dictionaries.data?.membership_statuses ?? [])}
          value={params.get('status') ?? ''}
          onChange={(event) => setParam('status', event.target.value)}
        />
      </Toolbar>
      {members.isPending ? (
        <Loading label="Загружаем членов клуба…" />
      ) : members.isError ? (
        <QueryError error={members.error} onRetry={() => void members.refetch()} />
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState title={filtered ? 'Никого не нашли' : 'В клубе пока нет карточек'} illustration={<Bone />}>
            {filtered && <p>Измените поиск или фильтр.</p>}
          </EmptyState>
        </Card>
      ) : (
        <>
          <Table caption="Члены клуба">
            <thead>
              <tr>
                <th scope="col">ФИО</th>
                <th scope="col">Членство</th>
                <th scope="col">В клубе с</th>
                <th scope="col">Дата рождения</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((member) => (
                <tr key={member.id}>
                  <td>
                    <Link to={`/members/${member.id}`}>{member.full_name}</Link>
                  </td>
                  <td>
                    <MembershipChip status={member.membership_status} />
                  </td>
                  <td className={tableStyles.nowrap}>{formatDate(member.membership_date)}</td>
                  <td className={tableStyles.nowrap}>{formatDate(member.date_of_birth)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
          {members.hasNextPage && (
            <Button variant="secondary" className={tableStyles.more} busy={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()}>
              Следующие 50
            </Button>
          )}
        </>
      )}
    </>
  );
}
