import type { ChipTone } from '../../shared/ui';
import type { Assessment, BreedingRequest, Mating } from './breedingApi';

export const requestStatuses: Record<BreedingRequest['status'], { label: string; tone: ChipTone }> = {
  pending: { label: 'Ожидает решения', tone: 'sun' },
  approved: { label: 'Одобрена', tone: 'lime' },
  rejected: { label: 'Отклонена', tone: 'pink' },
  cancelled: { label: 'Отменена', tone: 'sand' },
};

export const outcomes: Record<Mating['outcome'], { label: string; tone: ChipTone }> = {
  unknown: { label: 'Исход неизвестен', tone: 'sun' },
  no_litter: { label: 'Без помёта', tone: 'sand' },
  litter: { label: 'Помёт', tone: 'lime' },
};

export const blockingReasons: Record<Assessment['blocking_reasons'][number], string> = {
  same_dog: 'Выбрана одна и та же собака',
  female_inactive: 'Сука не на учёте',
  male_inactive: 'Кобель не на учёте',
  pair_sex: 'Пара не «сука и кобель»',
  different_breeds: 'Разные породы',
  pair_birth_dates: 'Дата вязки раньше рождения собаки',
  identifier_missing: 'У собаки нет идентификатора',
  female_medical_clearance: 'Нет медицинского допуска суки к вязке',
  male_medical_clearance: 'Нет медицинского допуска кобеля к вязке',
  parent_child: 'Родитель и потомок',
  shared_parent: 'Общий родитель',
  common_ancestor_3_generations: 'Общий предок в 3 поколениях',
  expected_birth_date_required: 'Нужна ожидаемая дата родов',
  litter_interval: 'Меньше 365 дней с прошлого помёта',
  expected_birth_before_mating: 'Ожидаемые роды раньше вязки',
  previous_outcome_unknown: 'Не указан исход прошлой вязки суки',
  another_approved_request: 'У суки уже есть одобренная непроведённая заявка',
};

export const warnings: Record<Assessment['warnings'][number], string> = {
  known_relationship: 'Дальнее родство',
  ancestry_incomplete: 'Родословная известна не полностью',
  hereditary_findings_require_expert: 'Диагнозы с наследственностью требуют оценки эксперта',
  earlier_litters_not_recorded: 'Прежние помёты в базе не записаны',
};

/** History actions per entity, as the server writes them. */
export const historyActions: Record<string, string> = {
  'request:created': 'Заявка создана',
  'request:corrected': 'Заявка исправлена',
  'request:cancelled': 'Заявка отменена',
  'request:approved': 'Эксперт одобрил заявку',
  'request:rejected': 'Эксперт отклонил заявку',
  'request:performed': 'Вязка проведена по заявке',
  'breeding:performed': 'Вязка проведена',
  'breeding:corrected': 'Вязка уточнена',
  'breeding:litter_recorded': 'Записан помёт',
  'litter:created': 'Помёт зарегистрирован',
  'litter:corrected': 'Помёт исправлен',
  'litter:puppy_registered': 'Зарегистрирован щенок',
  'litter:puppy_linked': 'Связана карточка щенка',
  'litter:puppy_unlinked': 'Связь со щенком снята',
};
