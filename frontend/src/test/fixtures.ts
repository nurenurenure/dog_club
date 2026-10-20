import type { components } from '../api/schema';

type Schemas = components['schemas'];

export const dogDictionaries: Schemas['DogDictionaries'] = {
  breeds: [
    { id: 1, name: 'Бигль' },
    { id: 2, name: 'Немецкая овчарка' },
  ],
  coat_colors: [{ id: 1, name: 'Трёхцветный' }],
  identifier_types: [{ id: 1, name: 'Микрочип' }],
  sexes: [
    { id: 1, name: 'Кобель' },
    { id: 2, name: 'Сука' },
  ],
  statuses: [
    { id: 1, name: 'На учёте', is_active: true },
    { id: 2, name: 'Потеряна', is_active: false },
  ],
  titles: [
    { id: 1, name: 'Юный чемпион' },
    { id: 2, name: 'Чемпион клуба' },
  ],
};

export const memberDictionaries: Schemas['MemberDictionaries'] = {
  address_types: [{ id: 1, name: 'Домашний' }],
  contact_types: [
    { id: 1, name: 'Телефон' },
    { id: 2, name: 'Email' },
  ],
  membership_statuses: [
    { id: 1, name: 'Активный' },
    { id: 2, name: 'Приостановлен' },
  ],
};

export function dog(overrides: Partial<Schemas['Dog']> = {}): Schemas['Dog'] {
  return {
    id: 7,
    nickname: 'Бублик',
    date_of_birth: '2023-05-04',
    breed: { id: 1, name: 'Бигль' },
    coat_color: { id: 1, name: 'Трёхцветный' },
    sex: { id: 1, name: 'Кобель' },
    status: { id: 1, name: 'На учёте' },
    identifier: { type: { id: 1, name: 'Микрочип' }, number: '643094100112358', date: null },
    height_cm: null,
    weight_kg: null,
    owner_ids: [3, 4],
    pedigree: {
      number: 'РКФ 1234567',
      issuer: 'РКФ',
      father: {
        dog_id: 5,
        dog: { id: 5, nickname: 'Граф', breed: { id: 1, name: 'Бигль' }, sex: { id: 1, name: 'Кобель' }, date_of_birth: '2019-01-01', pedigree_issuer: '', pedigree_number: '' },
        external: null,
      },
      mother: { dog_id: null, dog: null, external: { nickname: 'Lady Bee', pedigree_number: 'KC 42', issuer: 'Kennel Club' } },
    },
    titles: [{ type: { id: 1, name: 'Юный чемпион' }, awarded_on: '2024-08-18', basis: 'Диплом № 118' }],
    registration_date: '2023-06-12',
    version: 4,
    ...overrides,
  };
}

export function member(overrides: Partial<Schemas['Member']> = {}): Schemas['Member'] {
  return {
    id: 3,
    full_name: 'Мария Иванова',
    date_of_birth: '1990-02-01',
    membership_date: '2023-06-10',
    membership_status: { id: 1, name: 'Активный' },
    contacts: [{ id: 11, type: { id: 1, name: 'Телефон' }, value: '+7 900 000-00-00' }],
    addresses: [],
    version: 2,
    ...overrides,
  };
}
