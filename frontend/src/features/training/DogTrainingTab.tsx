import { Plus } from 'lucide-react';
import { ButtonLink } from '../../shared/ui';
import { SessionsTable } from './TrainingPage';
import { useOwnGrant } from './trainingApi';
import styles from './training.module.css';

/** Sessions of one dog; an instructor adds new ones while the dog is on the register. */
export function DogTrainingTab({ dogID, active }: { dogID: number; active: boolean }) {
  const grant = useOwnGrant();
  return (
    <>
      {grant.typeIDs.length > 0 && active && (
        <div className={styles.actions}>
          <ButtonLink to={`/training/new?dog=${dogID}`}>
            <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
            Внести занятие
          </ButtonLink>
        </div>
      )}
      <SessionsTable filter={{ dog_id: dogID }} hideDog />
    </>
  );
}
