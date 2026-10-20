import type { ReactNode, TableHTMLAttributes } from 'react';
import clsx from './clsx';
import styles from './Table.module.css';

/** Bordered table on white rows with a sand head; scrolls sideways on narrow screens. */
export function Table({ caption, className, children, ...rest }: TableHTMLAttributes<HTMLTableElement> & { caption: string }) {
  return (
    <div className={styles.scroll}>
      <table className={clsx(styles.table, className)} {...rest}>
        <caption className="visually-hidden">{caption}</caption>
        {children}
      </table>
    </div>
  );
}

/** Label and value tiles under a page header: owner, registration, titles, version. */
export function Facts({ children }: { children: ReactNode }) {
  return <dl className={styles.facts}>{children}</dl>;
}

export function Fact({ label, value, note, action }: { label: string; value: ReactNode; note?: ReactNode; action?: ReactNode }) {
  return (
    <div className={styles.fact}>
      <dt className={styles.factLabel}>{label}</dt>
      <dd className={styles.factValue}>{value}</dd>
      {note && <dd className={styles.factNote}>{note}</dd>}
      {action && <dd className={styles.factAction}>{action}</dd>}
    </div>
  );
}

/** Search and filters above a list. */
export function Toolbar({ children }: { children: ReactNode }) {
  return <div className={styles.toolbar}>{children}</div>;
}

export const tableStyles = styles;
