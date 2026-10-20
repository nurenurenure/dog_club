import type { ChipTone } from '../../shared/ui';
import type { Admission, Author, Clearance, MedicalStatus } from './vetApi';

export const decisions: Record<Admission['decision'], { label: string; tone: ChipTone }> = {
  allowed: { label: 'Допущена', tone: 'lime' },
  review_required: { label: 'Требует проверки', tone: 'sun' },
  denied: { label: 'Не допущена', tone: 'pink' },
  not_assessed: { label: 'Не оценено', tone: 'dashed' },
};

export const purposes: Record<Clearance['purpose'], string> = {
  events: 'Мероприятия',
  breeding: 'Вязка',
};

type ReasonCode = MedicalStatus['events']['reasons'][number]['code'];

export const reasons: Record<ReasonCode, string> = {
  clearance_missing: 'Нет заключения ветеринара.',
  clearance_voided: 'Последнее заключение отмечено ошибочным.',
  examination_voided: 'Осмотр, на котором основано заключение, отмечен ошибочным.',
  denied: 'Ветеринар не допустил собаку.',
  not_assessed: 'Ветеринар не дал оценку.',
  clearance_validity_unknown: 'Срок заключения неизвестен.',
  clearance_expired: 'Срок заключения истёк.',
  clearance_before_examination: 'Заключение датировано раньше осмотра.',
  vaccinations_missing: 'В заключении нет прививок-оснований.',
  vaccination_voided: 'Прививка-основание отмечена ошибочной.',
  vaccination_after_examination: 'Прививка-основание сделана после осмотра.',
  vaccination_validity_unknown: 'Срок прививки-основания неизвестен.',
  vaccination_expired: 'Срок прививки-основания истёк.',
  vaccination_expired_at_examination: 'Прививка-основание не действовала на дату осмотра.',
};

export const hereditary = (value: boolean | null) => (value === true ? 'наследственное' : value === false ? 'не наследственное' : 'наследственность не установлена');

/** Only the author changes a clinical record; others create their own. */
export function isMine(author: Author, userID: string): boolean {
  return author.user_id === userID;
}

/**
 * Lab 3: only «На учёте» is active. The veterinarian cannot read dog dictionaries,
 * so without them the status name decides; the server rejects records anyway.
 */
export function activeByName(status: { name: string }): boolean {
  return status.name === 'На учёте';
}
