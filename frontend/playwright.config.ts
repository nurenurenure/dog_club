import { defineConfig, devices } from '@playwright/test';
import { BASE_URL, OUT } from './e2e/support/env';

// One worker: each role signs in once for the whole run. The API rotates refresh
// tokens (a reused token revokes the session) and limits sign-ins per minute.
export default defineConfig({
  testDir: './e2e',
  outputDir: `${OUT}/results`,
  snapshotPathTemplate: '{testDir}/__screenshots__/{testFilePath}/{arg}{ext}',
  workers: 1,
  retries: 0,
  timeout: 5 * 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list'], ['html', { outputFolder: `${OUT}/report`, open: 'never' }]],
  use: {
    ...devices['Desktop Chrome'],
    baseURL: BASE_URL,
    viewport: { width: 1280, height: 800 },
    locale: 'ru-RU',
    timezoneId: 'Asia/Novosibirsk',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
});
