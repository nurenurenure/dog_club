import { Link } from 'react-router';
import { useSession } from '../../auth/AuthProvider';
import { Chip, type ChipTone } from '../../shared/ui';
import { useBreedingDog } from '../breeding/breedingApi';
import { useMember, useOwnMember } from '../members/membersApi';
import { usePatient } from '../veterinary/vetApi';
import type { Document, Template } from './documentsApi';

export const statuses: Record<Document['status'], { label: string; tone: ChipTone }> = {
  legacy: { label: 'Требует уточнения', tone: 'sun' },
  active: { label: 'Действует', tone: 'lime' },
  void: { label: 'Отменён', tone: 'dashed' },
};

export const categories: Record<NonNullable<Document['category']>, string> = {
  general: 'Общий',
  medical: 'Медицинский',
  breeding: 'Племенной',
};

/** Each PDF form belongs to exactly one document type, matched by its name. */
export const templates: Record<Template, string> = {
  member_card: 'Карточка члена клуба',
  dog_card: 'Карточка собаки',
  veterinary_conclusion: 'Ветеринарная справка',
  registration_certificate: 'Регистрационное свидетельство',
  breeding_conclusion: 'Заключение о вязке',
  litter_act: 'Акт о регистрации помёта',
  event_participants: 'Список участников мероприятия',
  event_protocol: 'Протокол мероприятия',
  event_diploma: 'Диплом',
};

export function templateFor(typeName: string): Template | undefined {
  return (Object.keys(templates) as Template[]).find((template) => templates[template] === typeName);
}

export const historyActions: Record<string, string> = {
  created: 'Запись создана',
  corrected: 'Реквизиты исправлены',
  file_added: 'Добавлен файл',
  voided: 'Документ отменён',
};

export function DocumentStatus({ document }: { document: Pick<Document, 'status' | 'expired'> }) {
  const item = statuses[document.status];
  return (
    <>
      <Chip tone={item.tone} struck={document.status === 'void'}>
        {item.label}
      </Chip>
      {document.expired && document.status !== 'void' && (
        <>
          {' '}
          <Chip tone="pink">Срок истёк</Chip>
        </>
      )}
    </>
  );
}

/** Dog names come from what the profile may read: the vet's patients or breeding cards. */
export function DocDogName({ id }: { id: number }) {
  const { profile } = useSession();
  return profile.role === 'veterinarian' ? <PatientName id={id} /> : <BreedingDogName id={id} />;
}

function PatientName({ id }: { id: number }) {
  const patient = usePatient(id);
  const name = patient.data?.nickname ?? (patient.isPending ? '…' : `собака №${id}`);
  return patient.data ? <Link to={`/patients/${id}`}>{name}</Link> : <>{name}</>;
}

function BreedingDogName({ id }: { id: number }) {
  const { profile } = useSession();
  const dog = useBreedingDog(id);
  const name = dog.data?.nickname ?? (dog.isPending ? '…' : `собака №${id}`);
  return dog.data && profile.role !== 'expert' ? <Link to={`/dogs/${id}`}>{name}</Link> : <>{name}</>;
}

/** Staff and leader read member cards; an owner recognises only themself. */
export function DocMemberName({ id }: { id: number }) {
  const { profile } = useSession();
  if (profile.role === 'staff' || profile.role === 'leader') return <OfficeMemberName id={id} />;
  if (profile.role === 'owner') return <OwnName id={id} />;
  return <>член клуба №{id}</>;
}

function OfficeMemberName({ id }: { id: number }) {
  const member = useMember(id);
  const name = member.data?.full_name ?? (member.isPending ? '…' : `член клуба №${id}`);
  return member.data ? <Link to={`/members/${id}`}>{name}</Link> : <>{name}</>;
}

function OwnName({ id }: { id: number }) {
  const own = useOwnMember();
  return <>{own.data?.id === id ? own.data.full_name : `член клуба №${id}`}</>;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} КБ`;
  return `${(bytes / 1024 / 1024).toFixed(1).replace('.', ',')} МБ`;
}
