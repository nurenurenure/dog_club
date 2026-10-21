import { Dog as DogIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import clsx from '../../shared/ui/clsx';
import { useDogPhoto } from './photoApi';
import styles from './dogs.module.css';

/** A dog photo filling its frame, or the dog icon while there is none. */
export function DogPhoto({ dogID, nickname, iconSize, className }: { dogID: number; nickname: string; iconSize: number; className?: string }) {
  const photo = useDogPhoto(dogID);
  const blob = photo.data?.blob;
  const [url, setUrl] = useState<string | null>(null);
  // One object URL per blob, released when the photo changes or the frame unmounts.
  useEffect(() => {
    if (!blob) {
      setUrl(null);
      return;
    }
    const next = URL.createObjectURL(blob);
    setUrl(next);
    return () => URL.revokeObjectURL(next);
  }, [blob]);
  return (
    <span className={clsx(styles.photoFrame, className)}>
      {url ? <img className={styles.photo} src={url} alt={`Фото: ${nickname}`} /> : <DogIcon size={iconSize} strokeWidth={1.6} aria-hidden="true" />}
    </span>
  );
}
