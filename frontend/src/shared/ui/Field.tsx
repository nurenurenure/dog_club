import { forwardRef, useId, type InputHTMLAttributes, type ReactNode, type TextareaHTMLAttributes } from 'react';
import clsx from './clsx';
import styles from './Field.module.css';

interface FieldProps {
  label: string;
  hint?: ReactNode;
  error?: string;
  aside?: ReactNode;
  children: (props: { id: string; describedBy?: string; invalid: boolean }) => ReactNode;
}

/** Label, control, hint and inline error with correct aria wiring. */
export function Field({ label, hint, error, aside, children }: FieldProps) {
  const id = useId();
  const hintID = `${id}-hint`;
  const errorID = `${id}-error`;
  const describedBy = [hint ? hintID : '', error ? errorID : ''].filter(Boolean).join(' ') || undefined;
  return (
    <div className={styles.field}>
      <div className={styles.labelRow}>
        <label htmlFor={id} className={styles.label}>
          {label}
        </label>
        {aside}
      </div>
      {children({ id, describedBy, invalid: Boolean(error) })}
      {hint && (
        <span id={hintID} className={styles.hint}>
          {hint}
        </span>
      )}
      {error && (
        <span id={errorID} className={styles.error}>
          {error}
        </span>
      )}
    </div>
  );
}

type InputProps = InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: ReactNode; error?: string; aside?: ReactNode };

export const TextField = forwardRef<HTMLInputElement, InputProps>(function TextField({ label, hint, error, aside, className, ...rest }, ref) {
  return (
    <Field label={label} hint={hint} error={error} aside={aside}>
      {({ id, describedBy, invalid }) => (
        <input ref={ref} id={id} aria-describedby={describedBy} aria-invalid={invalid || undefined} className={clsx(styles.control, className)} {...rest} />
      )}
    </Field>
  );
});

type AreaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string; hint?: ReactNode; error?: string };

export function TextArea({ label, hint, error, className, ...rest }: AreaProps) {
  return (
    <Field label={label} hint={hint} error={error}>
      {({ id, describedBy, invalid }) => (
        <textarea id={id} aria-describedby={describedBy} aria-invalid={invalid || undefined} className={clsx(styles.control, className)} {...rest} />
      )}
    </Field>
  );
}

export const MIN_PASSWORD = 15;

/** Counts Unicode code points, as the server does; no composition rules. */
export function passwordLength(value: string): number {
  return Array.from(value).length;
}

type PasswordProps = Omit<InputProps, 'type' | 'hint'> & { value: string; meter?: boolean };

export function PasswordField({ value, meter, ...rest }: PasswordProps) {
  const count = passwordLength(value);
  const done = count >= MIN_PASSWORD;
  const hint = meter ? (
    <>
      <span className={styles.meter}>
        <span className={styles.track} aria-hidden="true">
          <span className={clsx(styles.fill, done && styles.fillDone)} style={{ width: `${Math.min(100, (count / MIN_PASSWORD) * 100)}%` }} />
        </span>
        <span className={styles.count}>
          {Math.min(count, 999)} / {MIN_PASSWORD} символов
        </span>
      </span>
      <span>Не короче {MIN_PASSWORD} символов. Удобно — фраза из нескольких слов.</span>
    </>
  ) : undefined;
  return <TextField type="password" value={value} hint={hint} {...rest} />;
}

export const fieldStyles = styles;
