import { expect, test } from '@playwright/test'

// ConfigLLMEditor (tars#1286): add a provider and change a tier's model
// through the row-based table UI only — no JSON textarea involved — and
// confirm the save path (PATCH /v1/admin/config/values, same as every other
// settings field) actually persists both, while leaving the existing
// provider's api_key untouched.
test('adding a provider and changing a tier model through the LLM table UI persists on save', async ({ page, request }) => {
  test.setTimeout(60_000)
  const schema = () => request.get('/v1/admin/config/schema').then((r) => r.json())
  const original = await schema()
  const originalProviders = original.values.llm_providers
  const originalTiers = original.values.llm_tiers

  // try/finally, not a trailing statement: workers:1 means every spec in
  // this file shares one server + config file, so a thrown assertion
  // below must still restore the original providers/tiers before the
  // file's other specs (and setup-discovery.spec.ts, which assumes
  // "mock" is the pool's only, first provider) run against it.
  try {
    await page.addInitScript(() => localStorage.setItem('tars_console_locale', 'en'))
    await page.goto('/console/config/llm')

    // -- Add a provider row --
    await page.getByRole('button', { name: 'Add provider', exact: true }).click()
    const newProviderRow = page.locator('.llm-provider-row').last()
    await newProviderRow.getByLabel('Alias').fill('e2e-second')
    await newProviderRow.getByLabel('Alias').blur()
    await newProviderRow.getByLabel('Kind').selectOption('anthropic')
    const apiKeyInput = newProviderRow.getByLabel('API key')
    await apiKeyInput.fill('e2e-second-fake-credential')
    // Regression guard (tars#1286 PR review): a real CI Chrome's "suggest a
    // strong password" feature silently overwrote this field with its own
    // generated value after blur() when the input carried
    // autocomplete="new-password" — the DOM was still correct right here,
    // right after fill(), which is what isolated the bug to the field's
    // autocomplete value rather than to a submit-path logic error.
    // ConfigLLMEditor.svelte now matches ConfigControl.svelte's existing
    // autocomplete="off" convention for every other sensitive field on this
    // page.
    const domValueChecksum = await apiKeyInput.evaluate((el: HTMLInputElement) =>
      [...el.value].reduce((sum, ch) => sum + ch.charCodeAt(0), 0),
    )
    const expectedChecksum = [...'e2e-second-fake-credential'].reduce((sum, ch) => sum + ch.charCodeAt(0), 0)
    expect(domValueChecksum, `DOM input value checksum was ${domValueChecksum}, expected ${expectedChecksum}`).toBe(expectedChecksum)
    await apiKeyInput.blur()

    // No validation errors should remain under the Providers section once
    // the new row has an alias, a kind, and a key for its (default)
    // api-key mode.
    await expect(page.locator('#config-field-llm_providers')).not.toContainText('is required')

    // -- Change the "light" tier's model, leaving its provider alone --
    const lightTierRow = page.locator('.llm-tier-row', { hasText: 'light' })
    await lightTierRow.getByLabel('Model').fill('e2e-model-light-v2')
    await lightTierRow.getByLabel('Model').blur()

    await page.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(page.getByText('Saved; restart required to apply. Environment overrides take precedence.').first()).toBeVisible()

    const saved = await schema()
    const newProvider = saved.values.llm_providers['e2e-second']
    expect(newProvider).toMatchObject({ kind: 'anthropic', auth_mode: 'api-key' })
    // Checked as a boolean, not with the literal string in the matcher: a
    // string typed into a type="password" field gets scrubbed to asterisks
    // in Playwright's own diff output wherever it reappears later in the
    // run, which otherwise hides whether this genuinely matches or not
    // behind an unreadable "e2e-***...tial" vs "e2e-***...tial" diff. The
    // diagnostic on failure stays numeric so it cannot be scrubbed the
    // same way.
    const actualKey = typeof newProvider.api_key === 'string' ? newProvider.api_key : ''
    const checksum = [...actualKey].reduce((sum, ch) => sum + ch.charCodeAt(0), 0)
    expect(actualKey === 'e2e-second-fake-credential', `api_key length=${actualKey.length} charCodeSum=${checksum}`).toBe(true)
    // The pre-existing provider's real credential round-trips untouched —
    // there is no masking step to get wrong here (GET /schema already
    // returns it unmasked), but a stray clear/overwrite on an unrelated
    // row would still be a real regression.
    expect(saved.values.llm_providers.mock.api_key).toBe(originalProviders.mock.api_key)
    expect(saved.values.llm_tiers.light.model).toBe('e2e-model-light-v2')
    expect(saved.values.llm_tiers.light.provider).toBe('mock')
    // Untouched tiers are carried through unchanged, not dropped.
    expect(saved.values.llm_tiers.heavy.model).toBe(originalTiers.heavy.model)
    expect(saved.values.llm_tiers.standard.model).toBe(originalTiers.standard.model)
  } finally {
    await request.patch('/v1/admin/config/values', { data: { updates: { llm_providers: originalProviders, llm_tiers: originalTiers } } })
  }
})

test('an incomplete provider row blocks save with an inline error, not a silent drop', async ({ page, request }) => {
  const schema = () => request.get('/v1/admin/config/schema').then((r) => r.json())
  const original = await schema()
  const originalProviders = original.values.llm_providers

  await page.addInitScript(() => localStorage.setItem('tars_console_locale', 'en'))
  await page.goto('/console/config/llm')

  await page.getByRole('button', { name: 'Add provider', exact: true }).click()
  const newProviderRow = page.locator('.llm-provider-row').last()
  await newProviderRow.getByLabel('Alias').fill('e2e-incomplete')
  await newProviderRow.getByLabel('Alias').blur()
  // Kind is left unset — "pick a provider kind" should show under the
  // Providers section, and no PATCH should have gone out for it.
  await expect(page.locator('#config-field-llm_providers')).toContainText('pick a provider kind')
  await expect(page.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0)

  const unsaved = await schema()
  expect(unsaved.values.llm_providers).toEqual(originalProviders)
})
