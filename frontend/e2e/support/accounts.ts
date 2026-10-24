import { readFileSync } from 'node:fs';
import { ACCOUNTS } from './env';

export type Role = 'admin' | 'leader' | 'staff' | 'veterinarian' | 'expert' | 'owner';
export const ROLES: Role[] = ['admin', 'leader', 'staff', 'veterinarian', 'expert', 'owner'];

const titles: [string, Role][] = [
  ['Администратор', 'admin'],
  ['Руководитель клуба', 'leader'],
  ['Сотрудник клуба', 'staff'],
  ['Ветеринар', 'veterinarian'],
  ['Эксперт-кинолог', 'expert'],
  ['Владелец собаки', 'owner'],
];

export interface Account {
  login: string;
  title: string;
  role?: Role;
}

function read() {
  const lines = readFileSync(ACCOUNTS, 'utf8').split('\n');
  const password = /: (\S+)$/.exec(lines[1] ?? '')?.[1];
  if (!password) throw new Error(`No password in ${ACCOUNTS}: run scripts/club-data.py first`);
  const accounts: Account[] = [];
  for (const line of lines.slice(3)) {
    const match = /^(\S+)\s+(.+)$/.exec(line);
    if (!match) continue;
    const [, login, title] = match as unknown as [string, string, string];
    accounts.push({ login, title, role: titles.find(([prefix]) => title.startsWith(prefix))?.[1] });
  }
  return { password, accounts };
}

let cache: ReturnType<typeof read> | undefined;

export function password(): string {
  cache ??= read();
  return cache.password;
}

/** The first account the data script created for a role. */
export function accountFor(role: Role): Account {
  cache ??= read();
  const account = cache.accounts.find((item) => item.role === role);
  if (!account) throw new Error(`No ${role} account in ${ACCOUNTS}`);
  return account;
}

export function accountsWith(prefix: string): Account[] {
  cache ??= read();
  return cache.accounts.filter((item) => item.title.startsWith(prefix));
}
