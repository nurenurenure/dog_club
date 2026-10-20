import { Chip } from '../../shared/ui';
import { useDogDictionaries } from './dogsApi';

/** «На учёте» is lime; any inactive status is struck through on sand (lab 3). */
export function DogStatusChip({ status }: { status: { id: number; name: string } }) {
  const dictionaries = useDogDictionaries();
  const active = dictionaries.data?.statuses.find((item) => item.id === status.id)?.is_active ?? true;
  return active ? <Chip tone="lime">{status.name}</Chip> : <Chip struck>{status.name}</Chip>;
}
