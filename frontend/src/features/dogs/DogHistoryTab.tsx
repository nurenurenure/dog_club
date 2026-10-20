import { roleLabels } from '../../app/navigation';
import type { Role } from '../../api/session';
import { formatDateTime } from '../../shared/format';
import { Button, Card, EmptyState, Loading, QueryError, Timeline, tableStyles } from '../../shared/ui';
import { useDogHistory, type Dog, type DogChange } from './dogsApi';

const actions: Record<DogChange['action'], string> = {
  dog_created: 'Собака зарегистрирована',
  dog_updated: 'Изменена карточка',
  owners_updated: 'Изменены владельцы',
  pedigree_updated: 'Изменена родословная',
  title_recorded: 'Записан титул',
  title_corrected: 'Исправлена запись титула',
  title_record_corrected: 'Запись титула отмечена ошибочной',
};

const fields: Array<[keyof Dog, string]> = [
  ['nickname', 'кличка'],
  ['date_of_birth', 'дата рождения'],
  ['sex', 'пол'],
  ['breed', 'порода'],
  ['coat_color', 'окрас'],
  ['status', 'статус'],
  ['identifier', 'идентификатор'],
  ['height_cm', 'рост'],
  ['weight_kg', 'вес'],
  ['registration_date', 'дата регистрации'],
  ['owner_ids', 'владельцы'],
  ['pedigree', 'родословная'],
  ['titles', 'титулы'],
];

/** Names of the card parts that differ between two snapshots. */
export function changedFields(before: Dog | null, after: Dog): string[] {
  if (!before) return [];
  return fields.filter(([key]) => JSON.stringify(before[key] ?? null) !== JSON.stringify(after[key] ?? null)).map(([, label]) => label);
}

export function actorLabel(role: string): string {
  return roleLabels[role as Role] ?? role;
}

export function DogHistoryTab({ dogID }: { dogID: number }) {
  const history = useDogHistory(dogID);
  if (history.isPending) return <Loading />;
  if (history.isError) return <QueryError error={history.error} onRetry={() => void history.refetch()} />;
  const changes = history.data.pages.flatMap((page) => page.changes);
  return (
    <Card heading="История изменений">
      {changes.length === 0 ? (
        <EmptyState title="Изменений пока нет" />
      ) : (
        <Timeline
          entries={changes.map((change) => {
            const changed = changedFields(change.before, change.after);
            return {
              id: change.id,
              title: actions[change.action] ?? change.action,
              version: change.before ? `версия ${change.before.version} → ${change.after.version}` : `версия ${change.after.version}`,
              details: changed.length > 0 && change.action === 'dog_updated' ? `Изменено: ${changed.join(', ')}` : undefined,
              meta: `${actorLabel(change.actor_role)} · ${formatDateTime(change.changed_at)}`,
              reason: change.reason || undefined,
            };
          })}
        />
      )}
      {history.hasNextPage && (
        <Button variant="secondary" className={tableStyles.more} busy={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>
          Показать ещё
        </Button>
      )}
    </Card>
  );
}
