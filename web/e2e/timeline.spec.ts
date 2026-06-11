import { test, expect } from '@playwright/test';

// Helper: intercept POST /v1/auth/refresh to simulate an authenticated session.
// The token is stored in-memory (tokenStore), so cookie-based re-hydration is the
// only way to resolve 'loading' → 'authenticated' without a full login flow.
async function mockAuthRefresh(page: import('@playwright/test').Page) {
  await page.route('**/v1/auth/refresh', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ access_token: 'mock-token', expires_in: 3600 }),
    });
  });
}

test('home shows login prompt when anonymous', async ({ page }) => {
  // No auth intercept — refresh returns 401 → session stays anonymous.
  await page.route('**/v1/auth/refresh', async (route) => {
    await route.fulfill({ status: 401, body: '{}' });
  });
  await page.goto('/');
  await expect(page.getByRole('link', { name: /log in/i })).toBeVisible({ timeout: 5000 });
});

test('compose form renders for an authed session', async ({ page }) => {
  await mockAuthRefresh(page);
  await page.goto('/');
  await expect(page.getByLabel(/what's happening/i)).toBeVisible({ timeout: 5000 });
  await expect(page.getByRole('button', { name: /post/i })).toBeVisible();
});

test('notifications page renders against the mock', async ({ page }) => {
  await mockAuthRefresh(page);
  await page.goto('/notifications');
  await expect(page.getByRole('heading', { name: /notifications/i })).toBeVisible({ timeout: 5000 });
});
