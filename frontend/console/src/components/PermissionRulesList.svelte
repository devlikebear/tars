<script lang="ts">
  // Session Config → Permissions (#970): the "always allow" rules for the
  // session's folder. Rules are added only from approval cards; this lists
  // them and removes them.
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import { getSessionCwd, listPermissionRules, removePermissionRule, type PermissionRule } from '../lib/api'

  interface Props {
    sessionId: string
  }

  let { sessionId }: Props = $props()

  let dir = $state('')
  let rules = $state<PermissionRule[]>([])
  let loading = $state(true)
  let error = $state('')
  let removing = $state('')

  async function load() {
    loading = true
    error = ''
    try {
      const cwd = await getSessionCwd(sessionId)
      dir = cwd?.current?.trim() ?? ''
      rules = dir ? await listPermissionRules(dir) : []
    } catch {
      error = $t.chatApproval.rules.loadFailed
    } finally {
      loading = false
    }
  }

  async function remove(rule: PermissionRule) {
    const key = `${rule.provider}\n${rule.rule}`
    removing = key
    error = ''
    try {
      await removePermissionRule(dir, rule.provider, rule.rule)
      rules = rules.filter((r) => !(r.provider === rule.provider && r.rule === rule.rule))
    } catch {
      error = $t.chatApproval.rules.removeFailed
    } finally {
      removing = ''
    }
  }

  onMount(() => {
    void load()
  })
</script>

<section class="rules" aria-label={$t.chatApproval.rules.heading}>
  <h3 class="rules-heading">{$t.chatApproval.rules.heading}</h3>
  <p class="rules-hint">{$t.chatApproval.rules.hint}</p>
  {#if loading}
    <p class="rules-note">{$t.chatApproval.rules.loading}</p>
  {:else if !dir}
    <p class="rules-note">{$t.chatApproval.rules.noFolder}</p>
  {:else}
    <p class="rules-folder"><span class="rules-label">{$t.chatApproval.rules.folder}</span> <code title={dir}>{dir}</code></p>
    {#if rules.length === 0}
      <p class="rules-note">{$t.chatApproval.rules.empty}</p>
    {:else}
      <ul class="rules-list">
        {#each rules as rule (`${rule.provider}\n${rule.rule}`)}
          <li class="rules-row">
            <code class="rules-rule">{rule.rule}</code>
            <span class="rules-provider">{$t.chatApproval.rules.provider[rule.provider] ?? rule.provider}</span>
            <button
              type="button"
              class="btn btn-danger btn-sm"
              disabled={removing === `${rule.provider}\n${rule.rule}`}
              onclick={() => remove(rule)}
            >{$t.chatApproval.rules.remove}</button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
  {#if error}
    <p class="rules-error" role="alert">{error}</p>
  {/if}
</section>

<style>
  .rules {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    font-size: var(--text-sm);
    min-width: 0;
  }

  .rules-heading {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .rules-hint,
  .rules-note {
    margin: 0;
    color: var(--text-secondary);
  }

  .rules-folder {
    margin: 0;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .rules-label,
  .rules-provider {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .rules-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .rules-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    min-width: 0;
  }

  .rules-rule {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }

  .rules-error {
    margin: 0;
    color: var(--error);
    font-size: var(--text-xs);
  }
</style>
