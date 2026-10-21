import { useState } from 'react';
import { Button, FormDialog, ReasonDialog, TextField } from '../../shared/ui';
import { photoApi, photoProblem, usePhotoSaved, useDogPhoto } from './photoApi';
import styles from './dogs.module.css';

/** Staff uploads, replaces or removes the photo; a photo never changes the card version. */
export function PhotoControls({ dogID }: { dogID: number }) {
  const photo = useDogPhoto(dogID);
  const saved = usePhotoSaved();
  const [dialog, setDialog] = useState<'upload' | 'remove' | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const problem = photoProblem(file);
  const current = photo.data?.meta;
  if (photo.isPending) return null;

  return (
    <>
      <Button variant="secondary" className={styles.onForest} onClick={() => setDialog('upload')}>
        {current ? 'Заменить фото' : 'Загрузить фото'}
      </Button>
      {current && (
        <Button variant="link" className={styles.onForestLink} onClick={() => setDialog('remove')}>
          Убрать фото
        </Button>
      )}
      <FormDialog
        open={dialog === 'upload'}
        title={current ? 'Заменить фото' : 'Загрузить фото'}
        description="JPEG или PNG до 5 МиБ. Фото увидят сотрудники, руководитель, ветеринар, эксперт и владельцы собаки."
        confirmLabel="Сохранить"
        ready={Boolean(file && !problem)}
        onReload={() => void saved(dogID)}
        onClose={() => {
          setFile(null);
          setDialog(null);
        }}
        onSubmit={async () => {
          await photoApi.set(dogID, file!, current?.version ?? 0);
          await saved(dogID);
        }}
      >
        <TextField label="Фото" type="file" accept=".jpg,.jpeg,.png,image/jpeg,image/png" required onChange={(event) => setFile(event.target.files?.[0] ?? null)} error={problem} />
      </FormDialog>
      <ReasonDialog
        open={dialog === 'remove'}
        title="Убрать фото"
        description="Карточка снова покажет значок вместо фото. Действие останется в истории собаки."
        confirmLabel="Убрать"
        confirmVariant="danger"
        onReload={() => void saved(dogID)}
        onClose={() => setDialog(null)}
        onConfirm={async (reason) => {
          await photoApi.remove(dogID, current!.version, reason);
          await saved(dogID);
        }}
      />
    </>
  );
}
