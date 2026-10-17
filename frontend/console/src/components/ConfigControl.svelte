<script lang="ts">
  // One unified control per config field type (#1242): a toggle switch for
  // bool, a select for enum fields, and otherwise an always-visible input or
  // textarea — no more click-to-reveal "value" buttons. Height, font and
  // border come from app.css tokens via shared classes (.config-control-*),
  // not per-control CSS, so the search box and RemoteAccessCard can reuse
  // them too.
  import { untrack } from 'svelte'
  import { formatConfigDisplayValue } from '../lib/configStructured'
  import type { ConfigFieldMeta } from '../lib/types'

  interface ControlText {
    toggleOn: string
    toggleOff: string
    selectNone: string
    emptyPlaceholder: string
    jsonInvalid: string
    invalidValue: string
    listHint: string
  }

  interface Props {
    field: ConfigFieldMeta
    value: unknown
    dirty?: boolean
    text: ControlText
    onChange: (value: unknown) => void
  }

  let { field, value, dirty = false, text, onChange }: Props = $props()

  let error: string | null = $state(null)

  function toText(v: unknown): string {
    if (v === undefined || v === null) return ''
    if (field.type === 'string_list') {
      return Array.isArray(v) ? v.map((item) => String(item)).join('\n') : String(v)
    }
    if (field.type === 'json') {
      return typeof v === 'string' ? v : JSON.stringify(v ?? {}, null, 2)
    }
    return String(v)
  }

  // Sensitive text fields start blank (never reveal the masked server
  // value) with the masked summary as a placeholder; everything else shows
  // its live text. `syncedText` is the text we last adopted `value` from;
  // an external change (save-reload, revert-to-default) only overwrites
  // `draft` when the user has no uncommitted edit of their own pending.
  // `field` never changes identity for a mounted instance (callers key
  // their #each block on field.key), so these are deliberate one-time
  // snapshots, not missed reactivity — untrack() says so to svelte-check.
  let syncedText = untrack(() => toText(value))
  let draft = $state(untrack(() => (field.sensitive && field.type === 'string' ? '' : syncedText)))

  $effect(() => {
    const nextText = toText(value)
    if (nextText === syncedText) return
    const sensitiveText = field.sensitive && field.type === 'string'
    if (!sensitiveText && draft === syncedText) {
      draft = nextText
    }
    syncedText = nextText
  })

  function commit() {
    error = null
    if (field.type === 'bool' || field.type === 'select') return // committed immediately on change
    if (field.sensitive && draft === '') return // untouched secret; keep current value
    let parsed: unknown
    if (field.type === 'json') {
      try {
        parsed = draft.trim() === '' ? {} : JSON.parse(draft)
      } catch {
        error = text.jsonInvalid
        return
      }
    } else if (field.type === 'string_list') {
      parsed = draft.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean)
    } else if (field.type === 'int') {
      if (draft.trim() === '') { parsed = 0 } else {
        const n = Number(draft)
        if (!Number.isFinite(n) || !Number.isInteger(n)) { error = text.invalidValue; return }
        parsed = n
      }
    } else if (field.type === 'float') {
      if (draft.trim() === '') { parsed = 0 } else {
        const n = Number(draft)
        if (!Number.isFinite(n)) { error = text.invalidValue; return }
        parsed = n
      }
    } else {
      parsed = draft
    }
    onChange(parsed)
  }

  function handleKeydown(e: KeyboardEvent) {
    const multiline = field.type === 'string_list' || field.type === 'json'
    if (e.key === 'Enter' && (!multiline || e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      commit()
    }
  }

  function maskedPlaceholder(): string {
    if (!field.sensitive) return text.emptyPlaceholder
    const summary = formatConfigDisplayValue(value)
    return summary.kind === 'empty' ? text.emptyPlaceholder : '••••••••'
  }
</script>

{#if field.type === 'bool'}
  <button
    type="button"
    role="switch"
    aria-checked={!!value}
    class="config-control-toggle"
    class:on={!!value}
    class:dirty
    onclick={() => onChange(!value)}
  >
    <span class="config-control-toggle-dot"></span>
    <span class="config-control-toggle-label">{value ? text.toggleOn : text.toggleOff}</span>
  </button>
{:else if field.type === 'select' && field.options}
  <select
    class="config-control-input config-control-select"
    class:dirty
    value={String(value ?? '')}
    onchange={(e) => onChange((e.currentTarget as HTMLSelectElement).value)}
  >
    {#each field.options as opt}
      <option value={opt}>{opt || text.selectNone}</option>
    {/each}
  </select>
{:else if field.type === 'json' || field.type === 'string_list'}
  <div class="config-control-stack">
    <textarea
      class="config-control-input config-control-textarea"
      class:dirty
      class:invalid={!!error}
      placeholder={maskedPlaceholder()}
      bind:value={draft}
      onkeydown={handleKeydown}
      onblur={commit}
    ></textarea>
    {#if field.type === 'string_list'}
      <span class="config-control-hint">{text.listHint}</span>
    {/if}
    {#if error}<span class="config-control-error">{error}</span>{/if}
  </div>
{:else}
  <div class="config-control-stack">
    <input
      type={field.sensitive ? 'password' : (field.type === 'int' || field.type === 'float' ? 'number' : 'text')}
      step={field.type === 'float' ? '0.01' : undefined}
      class="config-control-input"
      class:dirty
      class:invalid={!!error}
      placeholder={maskedPlaceholder()}
      bind:value={draft}
      onkeydown={handleKeydown}
      onblur={commit}
      autocomplete="off"
    />
    {#if error}<span class="config-control-error">{error}</span>{/if}
  </div>
{/if}

<style>
  /* Shared button/input look (.config-control-*) lives in app.css — global
     on purpose, so RemoteAccessCard and the settings search box render the
     identical control (#1242) even though Svelte scopes component styles
     per file. Only this component's own layout stays local. */
  .config-control-stack {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .config-control-hint,
  .config-control-error {
    font-family: var(--font-body);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .config-control-error {
    color: var(--error);
  }

  .config-control-toggle-dot {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--text-ghost);
    transition: background var(--duration-fast, 120ms) var(--ease-out, ease);
  }

  .config-control-toggle.on .config-control-toggle-dot {
    background: var(--primary);
  }
</style>
