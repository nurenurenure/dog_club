import { X } from 'lucide-react';
import { useState } from 'react';
import { formatDate } from '../../shared/format';
import { Chip, SearchPicker } from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { useMembers } from './membersApi';
import styles from './members.module.css';

export interface PickedMember {
  id: number;
  full_name: string;
}

interface MemberPickerProps {
  label: string;
  value: PickedMember[];
  onChange: (value: PickedMember[]) => void;
  error?: string;
  hint?: string;
}

/** Several club members, e.g. the owners of a dog: search, add, remove. */
export function MemberPicker({ label, value, onChange, error, hint = 'ФИО члена клуба. Можно выбрать несколько.' }: MemberPickerProps) {
  const [query, setQuery] = useState('');
  const q = useDebounced(query.trim());
  const members = useMembers({ q, limit: 10 }, q.length > 0);
  const chosen = new Set(value.map((member) => member.id));
  const found = members.data?.pages.flatMap((page) => page.members).filter((member) => !chosen.has(member.id));
  return (
    <div className={styles.picker}>
      {value.length > 0 && (
        <ul className={styles.chosen} aria-label="Выбраны">
          {value.map((member) => (
            <li key={member.id}>
              <Chip tone="lime">
                {member.full_name}
                <button type="button" className={styles.remove} aria-label={`Убрать ${member.full_name}`} onClick={() => onChange(value.filter((item) => item.id !== member.id))}>
                  <X size={14} strokeWidth={2.4} aria-hidden="true" />
                </button>
              </Chip>
            </li>
          ))}
        </ul>
      )}
      <SearchPicker
        label={label}
        query={query}
        onQuery={setQuery}
        loading={members.isFetching || q !== query.trim()}
        error={error}
        hint={hint}
        items={found?.map((member) => ({
          id: member.id,
          title: member.full_name,
          note: [member.membership_status.name, member.date_of_birth && `род. ${formatDate(member.date_of_birth)}`].filter(Boolean).join(' · '),
        }))}
        onPick={(id) => {
          const member = found?.find((item) => item.id === id);
          if (member) {
            onChange([...value, { id: member.id, full_name: member.full_name }]);
            setQuery('');
          }
        }}
      />
    </div>
  );
}
