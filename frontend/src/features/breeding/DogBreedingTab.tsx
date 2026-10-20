import { HeartHandshake } from 'lucide-react';
import { useSession } from '../../auth/AuthProvider';
import { ButtonLink, Card } from '../../shared/ui';
import { LittersTable, MatingsTable, RequestsTable } from './BreedingPage';
import styles from './breeding.module.css';

/** Breeding records of one dog in its card; owners read their own dogs' records. */
export function DogBreedingTab({ dogID, active }: { dogID: number; active: boolean }) {
  const { profile } = useSession();
  const canSuggest = active && (profile.role === 'staff' || profile.role === 'leader');
  return (
    <>
      {canSuggest && (
        <Card className={styles.suggest}>
          <span>Кандидаты в пару с учётом породы, возраста, наследственных диагнозов и родства.</span>
          <ButtonLink to={`/breeding/candidates/${dogID}`} variant="secondary" size="small">
            <HeartHandshake size={16} strokeWidth={2} aria-hidden="true" />
            Подобрать пару
          </ButtonLink>
        </Card>
      )}
      <h2 className={styles.subheading}>Заявки</h2>
      <RequestsTable dogID={dogID} />
      <h2 className={styles.subheading}>Вязки</h2>
      <MatingsTable dogID={dogID} />
      <h2 className={styles.subheading}>Помёты</h2>
      <LittersTable dogID={dogID} />
    </>
  );
}
