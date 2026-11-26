<script lang="ts">
  // The Projects tab (docs/plans/vibe-mode-redesign unit 8,
  // screens/M-Projects): every vibe project on every paired desktop that
  // answers, in one list by the desk's own state — what waits on the
  // person first (the Waiting inbox folded in), then what the team makes,
  // what stopped, and what is finished. Each card is the console's own
  // (EasyProjectCard) with its menu: rename and archive. A phone cannot
  // delete a project, so the card offers no delete.
  import { t } from '../i18n'
  import EasyProjectCard from '../components/easy/EasyProjectCard.svelte'
  import { manageErrorKey } from '../lib/easyBoard'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import type { ProjectStore } from '../lib/mobile/projectStore.svelte'
  import { PHONE_FILTERS, filterCounts, phoneProjectKey, phoneSections, type PhoneFilter, type PhoneProject } from '../lib/mobile/projects'

  interface Props {
    projects: ProjectStore
    desktops: PairedDesktop[]
    // Desktops whose projects cannot be shown now (asleep, or no answer).
    missing: string[]
    onOpen: (desktopID: string, projectID: string) => void
    onNotice: (text: string) => void
  }

  let { projects, desktops, missing, onOpen, onNotice }: Props = $props()

  let filter = $state<PhoneFilter>('all')
  const counts = $derived(filterCounts(projects.projects))
  const sections = $derived(phoneSections(projects.projects, filter))
  // With more than one desktop, each card says which one it is on.
  const manyDesks = $derived(desktops.length > 1)

  function deskOf(p: PhoneProject): PairedDesktop | undefined {
    return desktops.find((d) => d.id === p.desktopID)
  }

  function errorText(code: string): string {
    if (code === 'unpaired') return $t.phone.computers.unpairedHint
    if (code === 'unreachable') return $t.phone.failures.unreachable
    return $t.easy.board.errors[manageErrorKey(code)]
  }

  async function rename(p: PhoneProject, name: string): Promise<string> {
    const desk = deskOf(p)
    if (!desk) return errorText('unreachable')
    const failure = await projects.rename(desk, p, name)
    return failure ? errorText(failure) : ''
  }

  async function archive(p: PhoneProject) {
    const desk = deskOf(p)
    if (!desk) return
    const failure = await projects.archive(desk, p)
    onNotice(failure ? errorText(failure) : $t.phone.projects.archived(p.name))
  }
</script>

<section class="projects" aria-label={$t.phone.projects.title}>
  <h1>{$t.phone.projects.title}</h1>

  {#if desktops.length === 0}
    <p class="empty" data-testid="phone-projects-nodesk">{$t.phone.projects.noComputer}</p>
  {:else}
    <div class="chips" role="tablist">
      {#each PHONE_FILTERS as f (f)}
        <button type="button" role="tab" class="chip" class:active={filter === f} aria-selected={filter === f} data-testid="phone-filter" data-filter={f} onclick={() => (filter = f)}>
          {$t.phone.projects.filters[f]} <span class="n">{counts[f]}</span>
        </button>
      {/each}
    </div>

    {#if !projects.loaded}
      <p class="empty">{$t.phone.projects.reading}</p>
    {:else if projects.projects.length === 0}
      <p class="empty" data-testid="phone-projects-empty">{$t.phone.projects.empty}</p>
    {:else if sections.length === 0}
      <p class="empty">{$t.phone.projects.emptyFilter}</p>
    {/if}

    {#each sections as { section, projects: list } (section)}
      <div class="section" data-testid="phone-section" data-section={section}>
        <h2 class="section-{section}">{$t.phone.projects.sections[section]}</h2>
        {#each list as p (phoneProjectKey(p))}
          <div class="item" data-testid="phone-project-item" data-desktop={p.desktopName}>
            <EasyProjectCard
              project={p}
              menu
              onNavigate={() => onOpen(p.desktopID, p.id)}
              onRename={(name) => rename(p, name)}
              onArchive={() => void archive(p)}
            />
            {#if manyDesks}<span class="desk">{p.desktopName}</span>{/if}
          </div>
        {/each}
      </div>
    {/each}

    {#if missing.length > 0}
      <p class="missing" data-testid="phone-projects-missing">{$t.phone.projects.notShown(missing.join(', '))}</p>
    {/if}
  {/if}
</section>

<style>
  .projects {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  h1 {
    margin: 0;
    font-size: var(--text-xl);
    font-family: var(--font-display);
  }
  .chips {
    display: flex;
    gap: var(--space-2);
    overflow-x: auto;
  }
  .chip {
    flex: none;
    min-height: 34px;
    padding: 0 var(--space-3);
    border-radius: 999px;
    border: 1px solid var(--border-default);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--text-sm);
  }
  .chip.active {
    background: var(--surface-elevated);
    color: var(--text-primary);
    border-color: var(--border-strong, var(--border-default));
  }
  .n {
    color: var(--text-tertiary);
    margin-left: 2px;
  }
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  h2 {
    margin: var(--space-2) 0 0;
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .section-your_turn {
    color: var(--warning);
  }
  .item {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  /* A card is a row to tap at phone width. */
  .item :global(.card) {
    min-height: 56px;
  }
  .item :global(.menu-button) {
    min-width: 40px;
    min-height: 40px;
  }
  .desk {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    padding-left: var(--space-3);
  }
  .empty,
  .missing {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.5;
  }
</style>
