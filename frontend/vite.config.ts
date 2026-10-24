import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// The API has no CORS: the dev server proxies /api so the browser sees one origin.
// Host is kept (no changeOrigin) so Go CrossOriginProtection accepts mutations.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: process.env.DOGCLUB_API_URL ?? 'http://127.0.0.1:8080' },
      '/health': { target: process.env.DOGCLUB_API_URL ?? 'http://127.0.0.1:8080' },
    },
  },
  css: {
    modules: { localsConvention: 'camelCaseOnly' },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    // Browser tests in e2e/ run under Playwright, not Vitest.
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['./src/test/setup.ts'],
    css: { modules: { classNameStrategy: 'non-scoped' } },
  },
});
