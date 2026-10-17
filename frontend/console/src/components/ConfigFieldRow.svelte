<script lang="ts">
  // One settings row: label + description + path on the left, the unified
  // control on the right (#1238/#1242) — replaces the old two-column
  // "quick-start-card" grid. `loc` is the already-localized label/
  // description (lib/configI18n.ts); this component only owns its own
  // static strings (badges, revert, env override notes).
  import { t } from '../i18n'
  import { buildConfigMetaBadges } from '../lib/configMetaBadges'
  import type { ConfigFieldLocalization, ConfigTextSegment } from '../lib/configSearch'
  import { formatConfigDisplayValue } from '../lib/configStructured'
  import ConfigControl from './ConfigControl.svelte'
  import type { ConfigEnvOverride, ConfigFieldMeta } from '../lib/types'

  interface Props {
    field: ConfigFieldMeta
    loc: ConfigFieldLocalization
    value: unknown
    dirty?: boolean
    labelSegments?: ConfigTextSegment[]
    descriptionSegments?: ConfigTextSegment[]
    envOverride?: ConfigEnvOverride | null
    envOverrideActiveValue?: string
    onChange: (value: unknown) => void
    onRevert?: () => void
  }

  let {
    field,
    loc,
    value,
    dirty = false,
    labelSegments,
    descriptionSegments,
    envOverride = null,
    envOverrideActiveValue,
    onChange,
    onRevert,
  }: Props = $props()

  let badges = $derived(buildConfigMetaBadges(field, value, dirty, {
    changed: $t.config.field.changedBadge,
    changedTitleDirty: $t.config.field.revertToDefault,
    changedTitleSaved: $t.config.field.revertToDefault,
    secret: $t.config.field.secretBadge,
    secretTitle: $t.config.field.secretBadge,
  }))

  let hasDefault = $derived(Object.prototype.hasOwnProperty.call(field, 'default_value'))
  let matchesDefault = $derived(hasDefault && !dirty && formatConfigDisplayValue(value).raw === formatConfigDisplayValue(field.default_value).raw)
  let canRevert = $derived(onRevert && hasDefault && !matchesDefault)
</script>

<div class="config-field-row" class:dirty id={`config-field-${field.key}`}>
  <div class="config-field-info">
    <div class="config-field-title-row">
      {#if labelSegments}
        <span class="config-field-label">{#each labelSegments as seg}{#if seg.matched}<mark>{seg.text}</mark>{:else}{seg.text}{/if}{/each}</span>
      {:else}
        <span class="config-field-label">{loc.label}</span>
      {/if}
      {#each badges as badge}
        <span class={`config-field-badge badge-${badge.tone}`} title={badge.title}>{badge.label}</span>
      {/each}
    </div>
    {#if descriptionSegments}
      <p class="config-field-description">{#each descriptionSegments as seg}{#if seg.matched}<mark>{seg.text}</mark>{:else}{seg.text}{/if}{/each}</p>
    {:else}
      <p class="config-field-description">{loc.description}</p>
    {/if}
    <span class="config-field-path">{field.path}</span>
    {#if envOverride}
      <span class="config-field-env" title={envOverrideActiveValue ? $t.config.field.envOverrideActive(envOverrideActiveValue) : ''}>
        {$t.config.field.envOverrideNote(envOverride.env_key)}
      </span>
    {/if}
  </div>
  <div class="config-field-control">
    <ConfigControl
      {field}
      {value}
      {dirty}
      onChange={onChange}
      text={{
        toggleOn: $t.config.field.toggleOn,
        toggleOff: $t.config.field.toggleOff,
        selectNone: $t.config.field.selectNone,
        emptyPlaceholder: $t.config.field.emptyPlaceholder,
        jsonInvalid: $t.config.field.jsonInvalid,
        invalidValue: $t.config.invalidValue,
        listHint: $t.config.field.listHint,
      }}
    />
    {#if canRevert}
      <button type="button" class="config-field-revert" onclick={() => onRevert?.()}>{$t.config.field.revertToDefault}</button>
    {/if}
  </div>
</div>

<style>
  .config-field-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(220px, 320px);
    gap: var(--space-4);
    padding: var(--space-3) var(--space-2);
    border-bottom: 1px solid var(--border-subtle);
    align-items: start;
  }

  .config-field-row:last-child {
    border-bottom: none;
  }

  .config-field-row.dirty {
    background: rgba(var(--primary-rgb), 0.04);
  }

  .config-field-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .config-field-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .config-field-label {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
  }

  .config-field-description {
    margin: 0;
    font-family: var(--font-body);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .config-field-path {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    overflow-wrap: anywhere;
  }

  .config-field-env {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--warning);
  }

  .config-field-badge {
    font-family: var(--font-body);
    font-size: var(--text-xs);
    padding: 1px 6px;
    border-radius: 999px;
    border: 1px solid var(--border-subtle);
  }

  .config-field-badge.badge-changed {
    color: var(--primary);
    border-color: color-mix(in srgb, var(--primary) 45%, var(--border-subtle));
  }

  .config-field-badge.badge-secret {
    color: var(--text-tertiary);
  }

  .config-field-control {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .config-field-revert {
    align-self: flex-end;
    background: none;
    border: none;
    padding: 0;
    color: var(--text-tertiary);
    font-family: var(--font-body);
    font-size: var(--text-xs);
    cursor: pointer;
    text-decoration: underline;
  }

  .config-field-revert:hover {
    color: var(--primary);
  }

  mark {
    background: rgba(var(--primary-rgb), 0.28);
    color: var(--text-primary);
    border-radius: 2px;
  }

  @media (max-width: 640px) {
    .config-field-row {
      grid-template-columns: 1fr;
    }
  }
</style>
