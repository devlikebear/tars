import test from 'node:test'
import assert from 'node:assert/strict'
import { emptyOnboardingForm } from '../src/lib/onboarding.ts'
import { autoApplyDiscovery, applyDiscoveredProvider, type SetupCandidate } from '../src/lib/setupDiscovery.ts'

const claude: SetupCandidate = {kind:'claude-code-cli', installed:true, ready:true, models:['opus','sonnet','haiku'], source:'cli', recommended:{heavy:'opus',standard:'sonnet',light:'haiku'}}
const codex: SetupCandidate = {kind:'openai-codex', installed:true, ready:true, models:['new-model'], source:'live', recommended:{heavy:'new-model',standard:'new-model',light:'new-model'}}

test('one ready provider prefills provider and all model tiers',()=>{
 const form=emptyOnboardingForm()
 assert.equal(autoApplyDiscovery(form,[claude,{...codex,ready:false}],false),true)
 assert.equal(form.provider.kind,'claude-code-cli')
 assert.equal(form.provider.auth_mode,'cli')
 assert.equal(form.tiers.standard.model,'sonnet')
 assert.equal(form.tiers.heavy.provider,'claude')
})
test('multiple ready providers wait for a choice',()=>{
 const form=emptyOnboardingForm()
 assert.equal(autoApplyDiscovery(form,[claude,codex],false),false)
 assert.equal(form.provider.kind,'')
 applyDiscoveredProvider(form,codex)
 assert.equal(form.provider.kind,'openai-codex')
 assert.equal(form.provider.auth_mode,'oauth')
 assert.equal(form.tiers.standard.model,'new-model')
})
test('re-entry and in-progress manual edits survive discovery',()=>{
 const form=emptyOnboardingForm()
 assert.equal(autoApplyDiscovery(form,[claude],true),false)
 form.provider.alias='my-choice'
 form.tiers.light.model='manual-model'
 const original=structuredClone(form)
 assert.equal(autoApplyDiscovery(form,[claude],false),false)
 assert.deepEqual(form,original)
})
test('unavailable provider cannot be selected',()=>{
 const form=emptyOnboardingForm()
 applyDiscoveredProvider(form,{...codex,ready:false})
 assert.equal(form.provider.kind,'')
})
