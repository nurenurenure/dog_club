import { Search } from 'lucide-react';
import type { ReactNode } from 'react';
import { TextField } from './Field';
import styles from './SearchPicker.module.css';

export interface PickerItem {
  id: number;
  title: string;
  note?: string;
}

interface SearchPickerProps {
  label: string;
  query: string;
  onQuery: (query: string) => void;
  items: PickerItem[] | undefined;
  loading: boolean;
  onPick: (id: number) => void;
  hint?: ReactNode;
  error?: string;
  empty?: string;
}

/** A search box with result buttons; results appear after the first typed character. */
export function SearchPicker({ label, query, onQuery, items, loading, onPick, hint, error, empty = 'Ничего не найдено.' }: SearchPickerProps) {
  const searching = query.trim().length > 0;
  return (
    <div className={styles.picker}>
      <TextField
        label={label}
        type="search"
        value={query}
        onChange={(event) => onQuery(event.target.value)}
        hint={hint}
        error={error}
        autoComplete="off"
        aside={<Search size={16} strokeWidth={2} aria-hidden="true" />}
      />
      {searching && (
        <div className={styles.results} aria-live="polite">
          {loading && !items ? (
            <p className={styles.note}>Ищем…</p>
          ) : items && items.length > 0 ? (
            <ul className={styles.list}>
              {items.map((item) => (
                <li key={item.id}>
                  <button type="button" className={styles.item} onClick={() => onPick(item.id)}>
                    <span className={styles.title}>{item.title}</span>
                    {item.note && <span className={styles.itemNote}>{item.note}</span>}
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className={styles.note}>{empty}</p>
          )}
        </div>
      )}
    </div>
  );
}
