import clsx from './clsx';
import type { ButtonHTMLAttributes } from 'react';
import { Link, type LinkProps } from 'react-router';
import styles from './Button.module.css';

export type ButtonVariant = 'primary' | 'save' | 'accent' | 'secondary' | 'link' | 'danger';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: 'small' | 'medium' | 'large';
  block?: boolean;
  busy?: boolean;
}

/** primary — one main action per screen; danger — always behind a reason dialog. */
export function Button({ variant = 'primary', size = 'medium', block, busy, disabled, type = 'button', className, children, ...rest }: ButtonProps) {
  return (
    <button
      type={type}
      className={clsx(styles.button, styles[variant], size !== 'medium' && styles[size], block && styles.block, className)}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      {...rest}
    >
      {children}
    </button>
  );
}

export const buttonStyles = styles;

interface ButtonLinkProps extends LinkProps {
  variant?: ButtonVariant;
  size?: 'small' | 'medium' | 'large';
}

/** Navigation that looks like a button: «Зарегистрировать собаку», «Изменить». */
export function ButtonLink({ variant = 'primary', size = 'medium', className, ...rest }: ButtonLinkProps) {
  return <Link className={clsx(styles.button, styles[variant], size !== 'medium' && styles[size], className)} {...rest} />;
}
