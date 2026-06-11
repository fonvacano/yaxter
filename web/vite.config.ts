import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// /v1 is proxied so the browser sees one origin — the deployed model
// (ALB routes /v1/* to api, everything else to the web bucket).
// When VITE_API_PROXY points to a real API server the full path is forwarded.
// When targeting prism (default: port 4010) the /v1 prefix is stripped because
// prism resolves paths from the OpenAPI `paths` object (relative to servers.url).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/v1': {
        target: process.env.VITE_API_PROXY ?? 'http://127.0.0.1:4010',
        changeOrigin: false,
        rewrite: process.env.VITE_API_PROXY
          ? undefined
          : (path) => path.replace(/^\/v1/, ''),
      },
    },
  },
  test: { environment: 'jsdom', globals: true, setupFiles: './src/test-setup.ts' },
});
