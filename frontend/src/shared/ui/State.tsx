import type { ReactNode } from 'react';
import styles from './State.module.css';

export function Loading({ label = 'Загрузка…' }: { label?: string }) {
  return (
    <div className={styles.state} role="status" aria-live="polite">
      {label}
    </div>
  );
}

export function EmptyState({ title, children, illustration }: { title: string; children?: ReactNode; illustration?: ReactNode }) {
  return (
    <div className={styles.state}>
      {illustration}
      <p className={styles.title}>{title}</p>
      {children}
    </div>
  );
}
