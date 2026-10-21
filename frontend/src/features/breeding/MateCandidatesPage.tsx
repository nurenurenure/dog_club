import { Link, useParams } from 'react-router';
import { useSession } from '../../auth/AuthProvider';
import { formatAge, formatDate } from '../../shared/format';
import { Alert, Ball, ButtonLink, Card, EmptyState, Loading, PageHeader, QueryError } from '../../shared/ui';
import clsx from '../../shared/ui/clsx';
import { DogPhoto } from '../dogs/DogPhoto';
import { useMateCandidates } from './breedingApi';
import { DogName } from './parts';
import styles from './breeding.module.css';

// Story cards alternate forest, lime and pink, as in the design system.
const tones = [styles.storyForest, styles.storyLime, styles.storyPink];

/** Lab 3 mate suggestions: a hint for staff, the request still gets the full check. */
export function MateCandidatesPage() {
  const dogID = Number(useParams().dogID);
  const { profile } = useSession();
  const candidates = useMateCandidates(dogID);
  return (
    <>
      <nav aria-label="Навигационная цепочка" className={styles.crumbs}>
        <Link to="/breeding">Племенная работа</Link> / Подбор пары
      </nav>
      <PageHeader eyebrow="Подбор пары" title="Кандидаты" />
      <p className={styles.lead}>
        Для собаки <DogName id={dogID} />
      </p>
      {candidates.isPending ? (
        <Loading label="Подбираем пару…" />
      ) : candidates.isError ? (
        <QueryError error={candidates.error} onRetry={() => void candidates.refetch()} />
      ) : (
        <>
          {candidates.data.truncated && <Alert tone="warning">Проверены первые 500 собак: уточните поиск породой, чтобы увидеть остальных.</Alert>}
          {candidates.data.candidates.length === 0 ? (
            <Card>
              <EmptyState title="Подходящих собак нет" illustration={<Ball />}>
                <p>Учитываются пол, порода, возраст 2–10 лет, наследственные диагнозы, родство и для сук — интервал между помётами.</p>
              </EmptyState>
            </Card>
          ) : (
            <ul className={styles.stories}>
              {candidates.data.candidates.map((dog, index) => {
                const female = dog.sex.name === 'Сука';
                const pair = female ? `female=${dog.id}&male=${dogID}` : `female=${dogID}&male=${dog.id}`;
                return (
                  <li key={dog.id} className={clsx(styles.story, tones[index % tones.length])}>
                    <DogPhoto dogID={dog.id} nickname={dog.nickname} iconSize={56} className={styles.storyPhoto} />
                    <span className={styles.storyName}>{dog.nickname}</span>
                    <span>
                      {dog.breed.name} · {dog.sex.name}
                    </span>
                    <span>
                      Род. {formatDate(dog.date_of_birth)} · {formatAge(dog.date_of_birth)}
                    </span>
                    <span className={styles.storyID}>{dog.identifier.number}</span>
                    {profile.role === 'staff' && (
                      <ButtonLink to={`/breeding/requests/new?${pair}`} size="small" variant="secondary" className={styles.storyAction}>
                        Создать заявку
                      </ButtonLink>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
          <p className={styles.note}>Подбор — подсказка на {formatDate(candidates.data.as_of)}: заявка всё равно пройдёт полную проверку пары.</p>
        </>
      )}
    </>
  );
}
