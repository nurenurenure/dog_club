import { Chip } from '../../shared/ui';
import { ACTIVE_MEMBERSHIP } from './membersApi';

export function MembershipChip({ status }: { status: { name: string } }) {
  return <Chip tone={status.name === ACTIVE_MEMBERSHIP ? 'lime' : 'sand'}>{status.name}</Chip>;
}
