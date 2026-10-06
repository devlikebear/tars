import { expect, test } from '@playwright/test'

test('complete provider edit resets optional fields and applies only after confirmed restart', async ({ page, request }) => {
 test.setTimeout(120_000)
 const schema = () => request.get('/v1/admin/config/schema').then(r => r.json())
 const original = await schema()
 const providers = original.values.llm_providers
 // An unused provider exercises the same persistence path without changing
 // the mock endpoint used by other tests or contacting a real provider.
 await request.patch('/v1/admin/config/values', {data:{updates:{llm_providers:{...providers, resettest:{kind:'openai',auth_mode:'oauth',base_url:'https://old.invalid/v1',api_key:'isolated-secret'}}}}})
 await page.addInitScript(() => localStorage.setItem('tars_console_locale','en'))
 await page.goto('/console/config')
 await page.getByRole('textbox',{name:'Search settings'}).fill('llm_providers')
 const card = page.locator('.quick-start-card').filter({hasText:'Named provider pool'})
 await card.getByRole('button',{name:'Click to edit',exact:true}).click()
 const current = await schema()
 await card.locator('textarea').fill(JSON.stringify({...current.values.llm_providers,resettest:{kind:'openai',api_key:current.values.llm_providers.resettest.api_key}}))
 await card.locator('textarea').blur()
 await page.getByRole('button',{name:'Save and apply',exact:true}).click()
 await expect(page.getByRole('button',{name:'Confirm restart',exact:true})).toBeVisible()
 const saved = await schema()
 expect(saved.values.llm_providers.resettest.auth_mode).toBe('api-key')
 expect(saved.values.llm_providers.resettest.base_url).toBe('https://api.openai.com/v1')
 expect(saved.values.llm_providers.resettest.api_key).toBe(current.values.llm_providers.resettest.api_key)
 expect(saved.runtime_started_at).toBe(original.runtime_started_at)
 expect(saved.runtime_values.llm_providers.resettest).toBeUndefined()
 await page.getByRole('button',{name:'Confirm restart',exact:true}).click()
 await expect.poll(async () => {try{return (await schema()).runtime_values.llm_providers.resettest?.auth_mode}catch{return ''}}, {timeout:60_000}).toBe('api-key')
 await request.patch('/v1/admin/config/values',{data:{updates:{llm_providers:providers}}})
})
