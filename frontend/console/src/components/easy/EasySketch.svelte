<script lang="ts">
  // A rough sketch of one blueprint screen: each part, from the fixed
  // vocabulary (EASY_SCREEN_PARTS), drawn as a simple shape with its words.
  import { t } from '../../i18n'
  import type { EasyScreen } from '../../lib/easy'

  let { screen, small = false }: { screen: EasyScreen; small?: boolean } = $props()
</script>

<figure class="sketch" class:small data-testid="easy-sketch">
  <div class="frame" aria-hidden="true">
    {#each screen.parts as part, i (i)}
      <div class="part part-{part.kind}">
        <span class="words">{part.text || $t.easy.workspace.blueprint.part[part.kind]}</span>
        {#if part.kind === 'list'}<span class="row"></span><span class="row"></span><span class="row"></span>{/if}
      </div>
    {/each}
  </div>
  <figcaption>
    <b>{screen.name}</b>
    {#if screen.line && !small}<span>{screen.line}</span>{/if}
  </figcaption>
</figure>

<style>
  .sketch {
    margin: 0;
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }
  .frame {
    min-height: 150px;
    max-height: 220px;
    overflow: hidden;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-3) var(--space-4);
    display: flex;
    flex-direction: column;
    gap: 7px;
    box-sizing: border-box;
    font-size: 11px;
    color: var(--text-tertiary);
  }
  .small .frame {
    min-height: 96px;
    max-height: 120px;
    padding: var(--space-2) var(--space-3);
    gap: 5px;
    font-size: 10px;
  }
  .part {
    min-width: 0;
  }
  .words {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .part-heading .words {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
  }
  .small .part-heading .words {
    font-size: 11px;
  }
  .part-field {
    border: 1px solid var(--border-default);
    border-radius: 3px;
    padding: 3px 6px;
  }
  .part-button {
    align-self: flex-start;
    background: rgba(var(--primary-rgb), 0.55);
    color: var(--primary-contrast);
    border-radius: 3px;
    padding: 3px 10px;
    max-width: 60%;
  }
  .part-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .row {
    height: 6px;
    border-radius: 2px;
    background: var(--surface-elevated);
  }
  .row:nth-child(3) {
    width: 80%;
  }
  .row:nth-child(4) {
    width: 60%;
  }
  .part-image {
    border: 1px dashed var(--border-strong);
    border-radius: 3px;
    padding: var(--space-3) 6px;
    text-align: center;
  }
  .part-link .words {
    color: var(--primary-text);
    text-decoration: underline;
  }
  figcaption {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--text-base);
  }
  figcaption b {
    font-weight: 600;
  }
  figcaption span {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .small figcaption {
    font-size: var(--text-sm);
  }
</style>
