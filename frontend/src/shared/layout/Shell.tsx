import { MoreHorizontal } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link, NavLink, Outlet, useLocation } from 'react-router';
import { homePath, navByRole, sections } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { Stamp } from '../ui';
import { ProfileSwitcher } from './ProfileSwitcher';
import styles from './Shell.module.css';

const BOTTOM_ITEMS = 4;

/** Sidebar on desktop; top bar and a bottom bar of four items (+ «Ещё») on phones. */
export function Shell() {
  const { profile } = useSession();
  const items = navByRole[profile.role];
  const location = useLocation();
  const [moreOpen, setMoreOpen] = useState(false);
  useEffect(() => setMoreOpen(false), [location.pathname]);

  const bottom = items.length > BOTTOM_ITEMS + 1 ? items.slice(0, BOTTOM_ITEMS) : items;
  const more = items.length > BOTTOM_ITEMS + 1 ? items.slice(BOTTOM_ITEMS) : [];
  const brand = (
    <Link to={homePath(profile.role)} className={styles.brand}>
      <Stamp size={44} />
      <span>
        Клуб
        <br />
        собаководства
      </span>
    </Link>
  );

  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        {brand}
        <nav aria-label="Разделы" className={styles.nav}>
          {items.map(({ key, label }) => {
            const { path, icon: Icon } = sections[key];
            return (
              <NavLink key={key} to={path} end={path === '/'} className={styles.navLink}>
                <Icon size={20} strokeWidth={2} aria-hidden="true" />
                {label}
              </NavLink>
            );
          })}
        </nav>
        <div className={styles.spacer} />
        <ProfileSwitcher placement="sidebar" />
      </aside>

      <div className={styles.main}>
        <header className={styles.topbar}>
          {brand}
          <ProfileSwitcher placement="topbar" />
        </header>
        <main className={styles.content} id="main">
          <Outlet />
        </main>
      </div>

      <nav aria-label="Разделы" className={styles.bottomNav}>
        {bottom.map(({ key, label }) => {
          const { path, icon: Icon } = sections[key];
          return (
            <NavLink key={key} to={path} end={path === '/'} className={styles.bottomLink}>
              <Icon size={22} strokeWidth={2} aria-hidden="true" />
              {label}
            </NavLink>
          );
        })}
        {more.length > 0 && (
          <button type="button" className={styles.bottomLink} aria-expanded={moreOpen} aria-controls="more-sections" onClick={() => setMoreOpen((open) => !open)}>
            <MoreHorizontal size={22} strokeWidth={2} aria-hidden="true" />
            Ещё
          </button>
        )}
      </nav>
      {moreOpen && (
        <div id="more-sections" className={styles.moreSheet}>
          {more.map(({ key, label }) => {
            const { path, icon: Icon } = sections[key];
            return (
              <NavLink key={key} to={path} className={styles.navLink}>
                <Icon size={20} strokeWidth={2} aria-hidden="true" />
                {label}
              </NavLink>
            );
          })}
        </div>
      )}
    </div>
  );
}
