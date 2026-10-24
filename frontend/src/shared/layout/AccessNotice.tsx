import { useState } from 'react';
import { reloadAccount } from '../../api/client';
import { useSession } from '../../auth/AuthProvider';
import { Button, EmptyState } from '../ui';
import styles from './Shell.module.css';

/** What blocks every page of this session, if anything: an owner without a member card. */
export function useAccessBlock(): 'member-unlinked' | null {
  const { user, profile } = useSession();
  if (profile.role === 'owner' && !user.club_member_id) return 'member-unlinked';
  return null;
}

export function AccessNotice({ block }: { block: 'member-unlinked' }) {
  return block === 'member-unlinked' ? <MemberUnlinked /> : null;
}

/** The owner profile works only through a member card that the administrator links to the account. */
function MemberUnlinked() {
  const [checking, setChecking] = useState(false);
  return (
    <EmptyState title="Карточка не привязана">
      <p>Карточка члена клуба не привязана к вашему аккаунту. Обратитесь к администратору клуба.</p>
      <div className={styles.noticeActions}>
        <Button
          variant="secondary"
          busy={checking}
          onClick={() => {
            setChecking(true);
            void reloadAccount().finally(() => setChecking(false));
          }}
        >
          Проверить снова
        </Button>
      </div>
    </EmptyState>
  );
}
