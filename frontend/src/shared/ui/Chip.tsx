import type { ReactNode } from 'react';
import clsx from './clsx';
import styles from './Chip.module.css';

export type ChipTone = 'lime' | 'sun' | 'pink' | 'ink' | 'sand' | 'dashed';

export function Chip({ tone = 'sand', struck, children }: { tone?: ChipTone; struck?: boolean; children: ReactNode }) {
  return <span className={clsx(styles.chip, styles[tone], struck && styles.struck)}>{children}</span>;
}
