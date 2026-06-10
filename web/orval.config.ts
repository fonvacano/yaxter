import { defineConfig } from 'orval';

export default defineConfig({
  yaxter: {
    input: '../api/openapi.yaml',
    output: {
      target: 'src/api/generated.ts',
      client: 'fetch',
      baseUrl: '/v1',
      override: {
        mutator: { path: 'src/api/fetcher.ts', name: 'customFetch' },
      },
    },
  },
});
