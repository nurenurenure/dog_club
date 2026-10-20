import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { ProblemAlert } from './Alert';
import { Button } from './Button';
import { TextArea } from './Field';
import styles from './Dialog.module.css';

export const MAX_REASON = 500;

interface ReasonDialogProps {
  open: boolean;
  title: string;
  description?: ReactNode;
  confirmLabel: string;
  /** Corrections and voids are recorded in history together with the reason. */
  onConfirm: (reason: string) => Promise<void>;
  onClose: () => void;
}

export function ReasonDialog({ open, title, description, confirmLabel, onConfirm, onClose }: ReasonDialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (open && !dialog.open) {
      setReason('');
      setError(null);
      dialog.showModal?.();
    }
    if (!open && dialog.open) dialog.close();
  }, [open]);

  const trimmed = reason.trim();
  const tooLong = Array.from(trimmed).length > MAX_REASON;
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!trimmed || tooLong) return;
    setBusy(true);
    setError(null);
    try {
      await onConfirm(trimmed);
      onClose();
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  };

  return (
    <dialog ref={ref} className={styles.dialog} onClose={onClose} aria-labelledby="reason-dialog-title">
      <form className={styles.body} onSubmit={submit}>
        <h2 id="reason-dialog-title" className={styles.title}>
          {title}
        </h2>
        {description && <p>{description}</p>}
        <TextArea
          label="Причина"
          required
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          hint="Сохранится в истории изменений. До 500 символов."
          error={tooLong ? 'Сократите причину до 500 символов.' : undefined}
        />
        {error ? <ProblemAlert error={error} /> : null}
        <div className={styles.actions}>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Отмена
          </Button>
          <Button type="submit" variant="save" busy={busy} disabled={!trimmed || tooLong}>
            {confirmLabel}
          </Button>
        </div>
      </form>
    </dialog>
  );
}
