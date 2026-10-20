import type { HTMLAttributes, ReactNode } from 'react';
import clsx from './clsx';
import styles from './Card.module.css';

interface CardProps extends HTMLAttributes<HTMLElement> {
  tone?: 'paper' | 'forest';
  heading?: ReactNode;
}

export function Card({ tone = 'paper', heading, className, children, ...rest }: CardProps) {
  return (
    <section className={clsx(styles.card, tone === 'forest' && styles.forest, className)} {...rest}>
      {heading && <h2 className={styles.title}>{heading}</h2>}
      {children}
    </section>
  );
}
