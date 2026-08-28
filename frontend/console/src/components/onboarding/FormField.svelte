<script lang="ts">
  import type { Snippet } from 'svelte'

  /**
   * The label + hint shell every onboarding form control sits in.
   *
   * The control itself is passed as a snippet rather than described by
   * props: the wizard's fields bind, dispatch, and validate in too many
   * different ways for a control-generating component to cover without
   * growing a prop per variant. Owning only the shell keeps every existing
   * `bind:value` and `oninput` handler exactly as it was, while the markup
   * and styles live in one place.
   *
   * The control stays a descendant of the <label>, so implicit label
   * association still holds and no `for`/`id` pairing is needed.
   */
  interface Props {
    label: string
    /** Secondary text shown after the label in a lighter weight. */
    hint?: string
    children: Snippet
  }

  let { label, hint, children }: Props = $props()
</script>

<label class="onboarding-field">
  <span>{label}{#if hint}&nbsp;<em>{hint}</em>{/if}</span>
  {@render children()}
</label>

<style>
  /*
   * These rules used to live in Onboarding.svelte behind :global() escapes,
   * because the markup they targeted was scattered across five child
   * components. With the shell owned here they can be scoped again.
   */
  .onboarding-field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: 14px;
  }
  .onboarding-field span {
    color: var(--text-muted);
    font-weight: 500;
  }
  .onboarding-field span em {
    color: var(--text-muted);
    font-weight: 400;
    font-style: normal;
    margin-left: 4px;
    font-size: 12px;
  }
  /*
   * Controls arrive as snippet children, so they carry the *caller's* style
   * scope, not this component's — these have to stay :global to reach them.
   * The nesting under .onboarding-field keeps that from leaking: it only
   * matches controls inside a FormField.
   */
  .onboarding-field :global(input),
  .onboarding-field :global(select),
  .onboarding-field :global(textarea) {
    padding: 8px 10px;
    border: 1px solid var(--border-soft);
    border-radius: 6px;
    background: var(--surface-1);
    color: var(--text-primary);
    font-family: inherit;
    font-size: 14px;
  }
  .onboarding-field :global(input:focus),
  .onboarding-field :global(select:focus),
  .onboarding-field :global(textarea:focus) {
    outline: 2px solid var(--primary);
    outline-offset: 1px;
  }
</style>
