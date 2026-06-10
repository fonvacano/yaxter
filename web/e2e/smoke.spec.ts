import { test, expect } from '@playwright/test';

test('register flow reaches the app shell', async ({ page }) => {
  await page.goto('/register');
  await page.getByLabel('Username').fill('newuser');
  await page.getByLabel('Email').fill('new@example.com');
  await page.getByLabel('Password').fill('password123');
  await page.getByRole('button', { name: /register/i }).click();
  // prism returns the contract's 201 example => token stored => home route.
  await expect(page).toHaveURL('/');
});

test('login page renders form and OAuth buttons from providers config', async ({ page }) => {
  await page.goto('/login');
  await expect(page.getByRole('form', { name: 'login form' })).toBeVisible();
  // prism serves a generated ProviderList; the container must render.
  await expect(page.getByTestId('oauth-providers')).toBeVisible();
});

test('unknown route shows not-found, home renders app shell', async ({ page }) => {
  await page.goto('/nowhere');
  await expect(page.getByText(/not found/i)).toBeVisible();
  await page.goto('/');
  // Home renders something meaningful (prism mock may hydrate session or not).
  // Check the app shell header is present.
  await expect(page.getByText('yaxter')).toBeVisible();
});
