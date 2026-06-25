import { test, expect } from '@playwright/test';
import { resetCategorization, seedRules, seedTransactions } from '../helpers/db';

// Scenarios from e2e/feat/rule-editor.feature
//
// The Rules page lists Rules read-only; create and edit both open the shared rule
// editor modal ([ADR-0016]). A no-handle save (the /rules surface passes none) closes
// the modal and refreshes the list with the re-categorized count; delete swaps the
// list in place; a validation error re-renders the editor body so the modal stays
// open. State is seeded directly into SQLite — there is no public write API.

test('Create a rule through the editor modal', async ({ page }) => {
  seedRules([]); // no rules: the empty-state baseline
  seedTransactions([{ id: 'txn-sbux', merchant: 'STARBUCKS', amount: 7 }]);

  await page.goto('/rules');
  await expect(page.getByTestId('rules-page')).toBeVisible();
  await expect(page.getByTestId('rules-empty')).toBeVisible();

  await page.getByTestId('rule-new').click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();

  await page.getByTestId('rule-editor-substring').fill('STARBUCKS');
  await page.getByTestId('rule-editor-classification').selectOption('spending');
  await page.getByTestId('rule-editor-category').selectOption('food_and_drink');
  await page.getByTestId('rule-editor-submit').click();

  // The modal closes and the list shows the new rule plus the re-categorized message.
  await expect(page.locator('dialog[open]')).toHaveCount(0);
  await expect(page.getByTestId('rule-row')).toHaveCount(1);
  await expect(page.getByTestId('rule-row-substring')).toHaveText('STARBUCKS');
  await expect(page.getByTestId('rules-feedback')).toContainText('re-categorized');
});

test('Edit a rule through the editor modal', async ({ page }) => {
  seedRules([
    { id: 'rule-1', merchantSubstring: 'WHOLEFOODS', classification: 'spending', categoryId: 'food_and_drink' },
  ]);

  await page.goto('/rules');
  await expect(page.getByTestId('rule-row-substring')).toHaveText('WHOLEFOODS');

  await page.getByTestId('rule-edit').click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();
  // Opens prefilled with the rule's current merchant text.
  await expect(page.getByTestId('rule-editor-substring')).toHaveValue('WHOLEFOODS');

  await page.getByTestId('rule-editor-substring').fill('WHOLE FOODS MARKET');
  await page.getByTestId('rule-editor-submit').click();

  await expect(page.locator('dialog[open]')).toHaveCount(0);
  await expect(page.getByTestId('rule-row-substring')).toHaveText('WHOLE FOODS MARKET');
});

test('Delete a rule', async ({ page }) => {
  seedRules([
    { id: 'rule-1', merchantSubstring: 'NETFLIX', classification: 'spending', categoryId: 'food_and_drink' },
  ]);

  await page.goto('/rules');
  await expect(page.getByTestId('rule-row')).toHaveCount(1);

  await page.getByTestId('rule-delete').click();

  await expect(page.getByTestId('rules-empty')).toBeVisible();
  await expect(page.getByTestId('rule-row')).toHaveCount(0);
});

test('A rule with a blank merchant keeps the editor open with an error', async ({ page }) => {
  resetCategorization();

  await page.goto('/rules');
  await page.getByTestId('rule-new').click();
  await expect(page.getByTestId('rule-editor')).toBeVisible();

  // Save with a blank merchant: the editor body re-renders inline and the modal stays.
  await page.getByTestId('rule-editor-category').selectOption('food_and_drink');
  await page.getByTestId('rule-editor-submit').click();

  await expect(page.getByTestId('rule-editor-error')).toBeVisible();
  await expect(page.locator('dialog[open]')).toHaveCount(1);
});
