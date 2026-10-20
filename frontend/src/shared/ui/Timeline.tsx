import type { ReactNode } from 'react';
import styles from './Timeline.module.css';

export interface TimelineEntry {
  id: number;
  title: ReactNode;
  version?: string;
  details?: ReactNode;
  meta: ReactNode;
  reason?: string;
}

/** Change history: what changed · version N → N+1, then author, date and reason. */
export function Timeline({ entries }: { entries: TimelineEntry[] }) {
  return (
    <ol className={styles.timeline}>
      {entries.map((entry) => (
        <li key={entry.id} className={styles.entry}>
          <span className={styles.dot} aria-hidden="true" />
          <p className={styles.title}>
            {entry.title}
            {entry.version && <span className={styles.version}> · {entry.version}</span>}
          </p>
          {entry.details && <p className={styles.details}>{entry.details}</p>}
          <p className={styles.meta}>
            {entry.meta}
            {entry.reason && <> · «{entry.reason}»</>}
          </p>
        </li>
      ))}
    </ol>
  );
}
