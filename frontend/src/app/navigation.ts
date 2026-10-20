import {
  Activity,
  BarChart3,
  CalendarDays,
  ClipboardList,
  Dog,
  FileText,
  GraduationCap,
  HeartHandshake,
  Home,
  IdCard,
  type LucideIcon,
  ShieldCheck,
  Stethoscope,
  Syringe,
  Users,
} from 'lucide-react';
import type { Role } from '../api/session';

export type SectionKey =
  | 'home'
  | 'members'
  | 'dogs'
  | 'breeding'
  | 'events'
  | 'training'
  | 'documents'
  | 'reports'
  | 'card'
  | 'patients'
  | 'vaccines'
  | 'users'
  | 'instructors'
  | 'system';

export interface Section {
  path: string;
  icon: LucideIcon;
  /** Step of the frontend plan where the screen is implemented. */
  step: number;
}

export const sections: Record<SectionKey, Section> = {
  home: { path: '/', icon: Home, step: 8 },
  members: { path: '/members', icon: Users, step: 3 },
  dogs: { path: '/dogs', icon: Dog, step: 3 },
  breeding: { path: '/breeding', icon: HeartHandshake, step: 5 },
  events: { path: '/events', icon: CalendarDays, step: 6 },
  training: { path: '/training', icon: GraduationCap, step: 7 },
  documents: { path: '/documents', icon: FileText, step: 7 },
  reports: { path: '/reports', icon: BarChart3, step: 7 },
  card: { path: '/me', icon: IdCard, step: 3 },
  patients: { path: '/patients', icon: Stethoscope, step: 4 },
  vaccines: { path: '/vaccines', icon: Syringe, step: 4 },
  users: { path: '/admin/users', icon: ClipboardList, step: 8 },
  instructors: { path: '/admin/instructors', icon: ShieldCheck, step: 8 },
  system: { path: '/admin/system', icon: Activity, step: 8 },
};

export interface NavItem {
  key: SectionKey;
  label: string;
}

const staffNav: NavItem[] = [
  { key: 'home', label: 'Обзор' },
  { key: 'members', label: 'Члены клуба' },
  { key: 'dogs', label: 'Собаки' },
  { key: 'breeding', label: 'Племенная работа' },
  { key: 'events', label: 'Мероприятия' },
  { key: 'training', label: 'Дрессировка' },
  { key: 'documents', label: 'Документы' },
  { key: 'reports', label: 'Отчёты' },
];

/** One active profile per session: the menu follows its role only. */
export const navByRole: Record<Role, NavItem[]> = {
  owner: [
    { key: 'home', label: 'Главная' },
    { key: 'dogs', label: 'Мои собаки' },
    { key: 'events', label: 'Мероприятия' },
    { key: 'training', label: 'Дрессировка' },
    { key: 'documents', label: 'Документы' },
    { key: 'card', label: 'Моя карточка' },
  ],
  staff: staffNav,
  leader: staffNav,
  veterinarian: [
    { key: 'patients', label: 'Пациенты' },
    { key: 'vaccines', label: 'Препараты' },
    { key: 'documents', label: 'Медицинские документы' },
  ],
  expert: [
    { key: 'events', label: 'Мои мероприятия' },
    { key: 'breeding', label: 'Племенные заявки' },
    { key: 'documents', label: 'Документы' },
  ],
  admin: [
    { key: 'users', label: 'Пользователи' },
    { key: 'instructors', label: 'Допуски инструкторов' },
    { key: 'system', label: 'Состояние системы' },
  ],
};

export const roleLabels: Record<Role, string> = {
  admin: 'Администратор',
  staff: 'Сотрудник клуба',
  veterinarian: 'Ветеринар',
  expert: 'Эксперт-кинолог',
  owner: 'Владелец собаки',
  leader: 'Руководитель клуба',
};

export function homePath(role: Role): string {
  return sections[navByRole[role][0]!.key].path;
}

export function navItem(role: Role, key: SectionKey): NavItem | undefined {
  return navByRole[role].find((item) => item.key === key);
}
