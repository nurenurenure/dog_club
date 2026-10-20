import { useState } from 'react';
import { formatDate } from '../../shared/format';
import { SearchPicker } from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { useDogs, type DogSummary } from './dogsApi';

interface DogPickerProps {
  label: string;
  /** Mother and father are searched among dogs of one sex on the server. */
  sexID?: number;
  excludeID?: number;
  onPick: (dog: DogSummary) => void;
  error?: string;
}

export function DogPicker({ label, sexID, excludeID, onPick, error }: DogPickerProps) {
  const [query, setQuery] = useState('');
  const q = useDebounced(query.trim());
  const dogs = useDogs({ q, sex_id: sexID, limit: 10 }, q.length > 0);
  const found = dogs.data?.pages.flatMap((page) => page.dogs).filter((dog) => dog.id !== excludeID);
  return (
    <SearchPicker
      label={label}
      query={query}
      onQuery={setQuery}
      loading={dogs.isFetching || q !== query.trim()}
      error={error}
      hint="Кличка, номер идентификатора или родословной."
      items={found?.map((dog) => ({
        id: dog.id,
        title: dog.nickname,
        note: `${dog.breed.name} · ${dog.sex.name} · род. ${formatDate(dog.date_of_birth)} · ${dog.identifier.number}`,
      }))}
      onPick={(id) => {
        const dog = found?.find((item) => item.id === id);
        if (dog) {
          onPick(dog);
          setQuery('');
        }
      }}
    />
  );
}
