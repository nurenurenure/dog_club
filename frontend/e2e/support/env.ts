/** Where the tests run: a local stand by default, the Compose network under scripts/e2e.py. */
export const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:5174';
/** Logins and the shared password written by scripts/club-data.py. */
export const ACCOUNTS = process.env.E2E_ACCOUNTS ?? '../var/club-accounts.txt';
/** Reports, traces and failure screenshots; var/ stays outside Git. */
export const OUT = process.env.E2E_OUT ?? '../var/e2e';
export const MAILPIT_URL = process.env.MAILPIT_URL ?? 'http://localhost:28025';
