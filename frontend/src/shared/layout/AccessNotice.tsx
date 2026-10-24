import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { reloadAccount } from '../../api/client';
import { errorMessage } from '../../api/problem';
import { homePath, roleLabels } from '../../app/navigation';
import { useAuth, useSession } from '../../auth/AuthProvider';
import { listProfiles } from '../../auth/authApi';
import { Alert, Button, EmptyState, Loading } from '../ui';
import styles from './Shell.module.css';

/** What blocks every page of this session, if anything: a disabled profile or an owner without a member card. */
export function useAccessBlock(): 'profile-disabled' | 'member-unlinked' | null {
  const { user, profile } = useSession();
  if (!profile.enabled) return 'profile-disabled';
  if (profile.role === 'owner' && !user.club_member_id) return 'member-unlinked';
  return null;
}

export function AccessNotice({ block }: { block: 'profile-disabled' | 'member-unlinked' }) {
  return block === 'profile-disabled' ? <ProfileDisabled /> : <MemberUnlinked />;
}

/** The administrator disabled the current profile: offer the account's other enabled profiles. */
function ProfileDisabled() {
  const { profile } = useSession();
  const { switchProfile, logout } = useAuth();
  const navigate = useNavigate();
  const profiles = useQuery({ queryKey: ['auth', 'profiles'], queryFn: listProfiles });
  const [failure, setFailure] = useState('');
  const others = (profiles.data?.profiles ?? []).filter((item) => item.enabled && item.id !== profile.id);

  const choose = async (id: string, role: keyof typeof roleLabels) => {
    setFailure('');
    try {
      await switchProfile(id);
      navigate(homePath(role));
    } catch (error) {
      setFailure(errorMessage(error));
    }
  };

  return (
    <EmptyState title={`Профиль «${roleLabels[profile.role]}» отключён администратором`}>
      {profiles.isPending ? (
        <Loading label="Загрузка профилей…" />
      ) : others.length > 0 ? (
        <p>Выберите другой профиль этого аккаунта.</p>
      ) : (
        <p>Других включённых профилей у аккаунта нет. Обратитесь к администратору клуба.</p>
      )}
      <div className={styles.noticeActions}>
        {others.map((item) => (
          <Button key={item.id} variant="secondary" onClick={() => void choose(item.id, item.role)}>
            {roleLabels[item.role]}
          </Button>
        ))}
        <Button variant="link" onClick={() => void logout().catch(() => undefined)}>
          Выйти
        </Button>
      </div>
      {failure && <Alert tone="error">{failure}</Alert>}
    </EmptyState>
  );
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
