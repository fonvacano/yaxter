import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// /v1 is proxied so the browser sees one origin — the deployed model
// (ALB routes /v1/* to api, everything else to the web bucket).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/v1': {
        target: process.env.VITE_API_PROXY ?? 'http://localhost:4010',
        changeOrigin: false,
      },
    },
  },
});
