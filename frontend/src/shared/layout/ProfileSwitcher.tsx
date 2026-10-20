import { useQuery } from '@tanstack/react-query';
import { Check, ChevronDown } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { errorMessage } from '../../api/problem';
import { homePath, roleLabels } from '../../app/navigation';
import { useAuth, useSession } from '../../auth/AuthProvider';
import { listProfiles } from '../../auth/authApi';
import clsx from '../ui/clsx';
import styles from './ProfileSwitcher.module.css';

function initials(login: string): string {
  const letters = login.replace(/[^\p{L}\p{N}]+/gu, ' ').trim().split(/\s+/);
  const value = letters.length > 1 ? `${letters[0]![0]}${letters[1]![0]}` : login.slice(0, 2);
  return value.toUpperCase();
}

/** Account menu: other profiles of this account, account settings, logout. */
export function ProfileSwitcher({ placement }: { placement: 'sidebar' | 'topbar' }) {
  const { user, profile } = useSession();
  const { switchProfile, logout } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [failure, setFailure] = useState('');
  const wrap = useRef<HTMLDivElement>(null);
  const profiles = useQuery({ queryKey: ['auth', 'profiles'], queryFn: listProfiles, enabled: open });

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent ? event.key === 'Escape' : !wrap.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', close);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', close);
    };
  }, [open]);

  const choose = async (id: string, role: keyof typeof roleLabels) => {
    setFailure('');
    try {
      await switchProfile(id);
      setOpen(false);
      navigate(homePath(role));
    } catch (error) {
      setFailure(errorMessage(error));
    }
  };

  const others = profiles.data?.profiles ?? [];
  return (
    <div className={styles.wrap} ref={wrap}>
      <button
        type="button"
        className={clsx(styles.trigger, placement === 'topbar' && styles.compact)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={placement === 'topbar' ? `${user.login}, ${roleLabels[profile.role]}: меню аккаунта` : undefined}
        onClick={() => setOpen((value) => !value)}
      >
        <span className={styles.avatar} aria-hidden="true">
          {initials(user.login)}
        </span>
        {placement === 'sidebar' && (
          <>
            <span className={styles.who}>
              <span className={styles.login}>{user.login}</span>
              <span className={styles.role}>{roleLabels[profile.role]}</span>
            </span>
            <ChevronDown size={16} aria-hidden="true" />
          </>
        )}
      </button>
      {open && (
        <div role="menu" aria-label="Аккаунт" className={clsx(styles.menu, placement === 'sidebar' ? styles.menuSidebar : styles.menuTopbar)}>
          <span className={styles.group}>Профили · права не складываются</span>
          {profiles.isPending && <span className={styles.status}>Загрузка профилей…</span>}
          {profiles.isError && <span className={styles.status}>{errorMessage(profiles.error)}</span>}
          {others.map((item) => {
            const current = item.id === profile.id;
            return (
              <button
                key={item.id}
                type="button"
                role="menuitemradio"
                aria-checked={current}
                className={styles.item}
                disabled={current || !item.enabled}
                onClick={() => void choose(item.id, item.role)}
              >
                <span>
                  {roleLabels[item.role]}
                  {!item.enabled && ' · отключён'}
                </span>
                {current && <Check size={18} aria-hidden="true" />}
              </button>
            );
          })}
          {failure && (
            <span className={styles.status} role="alert">
              {failure}
            </span>
          )}
          <span className={styles.divider} />
          <Link role="menuitem" to="/account" className={styles.item} onClick={() => setOpen(false)}>
            Аккаунт и пароль
          </Link>
          <button type="button" role="menuitem" className={styles.item} onClick={() => void logout().catch(() => undefined)}>
            Выйти
          </button>
        </div>
      )}
    </div>
  );
}
