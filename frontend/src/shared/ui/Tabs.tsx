import { useRef, type KeyboardEvent } from 'react';
import styles from './Tabs.module.css';

interface TabsProps<T extends string> {
  label: string;
  value: T;
  options: ReadonlyArray<{ value: T; label: string }>;
  onChange: (value: T) => void;
  idPrefix: string;
}

/** Segmented control with the WAI-ARIA tabs keyboard pattern (arrows, Home, End). */
export function Tabs<T extends string>({ label, value, options, onChange, idPrefix }: TabsProps<T>) {
  const refs = useRef<Array<HTMLButtonElement | null>>([]);
  const move = (event: KeyboardEvent, index: number) => {
    const last = options.length - 1;
    const next = { ArrowRight: index + 1 > last ? 0 : index + 1, ArrowLeft: index - 1 < 0 ? last : index - 1, Home: 0, End: last }[event.key];
    if (next === undefined) return;
    event.preventDefault();
    onChange(options[next]!.value);
    refs.current[next]?.focus();
  };
  return (
    <div role="tablist" aria-label={label} className={styles.tabs}>
      {options.map((option, index) => (
        <button
          key={option.value}
          ref={(node) => {
            refs.current[index] = node;
          }}
          type="button"
          role="tab"
          id={`${idPrefix}-tab-${option.value}`}
          aria-controls={`${idPrefix}-panel-${option.value}`}
          aria-selected={option.value === value}
          tabIndex={option.value === value ? 0 : -1}
          className={styles.tab}
          onClick={() => onChange(option.value)}
          onKeyDown={(event) => move(event, index)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

export function tabPanelProps(idPrefix: string, value: string) {
  return { role: 'tabpanel', id: `${idPrefix}-panel-${value}`, 'aria-labelledby': `${idPrefix}-tab-${value}` } as const;
}
