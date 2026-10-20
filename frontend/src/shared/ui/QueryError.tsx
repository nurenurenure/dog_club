import { Link } from 'react-router';
import { ApiError } from '../../api/problem';
import { useSession } from '../../auth/AuthProvider';
import { ProblemAlert } from './Alert';
import { Button } from './Button';
import { Card } from './Card';
import { Bone } from './Illustration';
import { EmptyState } from './State';

/** A failed load: inactive membership for owners, a missing record, or a retryable problem. */
export function QueryError({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { profile } = useSession();
  const status = error instanceof ApiError ? error.status : 0;
  if (status === 403 && profile.role === 'owner') {
    return (
      <Card>
        <EmptyState title="Членство неактивно" illustration={<Bone />}>
          <p>Раздел владельца открывается при активном членстве в клубе. Обратитесь к сотруднику клуба. Аккаунт и другие профили доступны в меню.</p>
        </EmptyState>
      </Card>
    );
  }
  if (status === 404) {
    return (
      <Card>
        <EmptyState title="Запись не найдена или недоступна" illustration={<Bone />}>
          <Link to="/">На главную</Link>
        </EmptyState>
      </Card>
    );
  }
  return (
    <div>
      <ProblemAlert error={error} />
      {onRetry && (
        <Button variant="secondary" size="small" onClick={onRetry} style={{ marginTop: 'var(--space-3)' }}>
          Повторить
        </Button>
      )}
    </div>
  );
}
