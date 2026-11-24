<script lang="ts">
  // Easy mode's make screen (docs/decisions/vibe-coder-identity.md §4): one
  // box to say what to make, and the projects made so far. Starting a
  // project is POST /v1/easy/projects; the server picks the folder, sets the
  // stack up and opens the team's work, so the next screen is the project.
  //
  // While this computer lacks what a project needs (GET /v1/easy/readiness),
  // the Start screen takes the make box's place and the answer is read again
  // every few seconds, and whenever the tab comes back into view, until it
  // is ready. A server that cannot answer leaves the make box as it was.
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import { createEasyProject, getEasyReadiness, listEasyProjects } from '../../lib/api/easy'
  import { APIRequestError } from '../../lib/api/client'
  import { easyProjectPath, startPollMs, type EasyProject, type EasyReadiness } from '../../lib/easy'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'
  import EasyStart from './EasyStart.svelte'

  let { onNavigate }: { onNavigate: (path: string) => void } = $props()

  let idea = $state('')
  let starting = $state(false)
  let errorCode = $state('')
  let projects = $state<EasyProject[]>([])
  let loadFailed = $state(false)
  let readiness = $state<EasyReadiness | null>(null)
  let checking = $state(false)
  let pollTimer: ReturnType<typeof setTimeout> | undefined

  async function check() {
    if (checking) return
    checking = true
    clearTimeout(pollTimer)
    try {
      readiness = await getEasyReadiness()
    } catch {
      readiness = null
    } finally {
      checking = false
    }
    if (readiness && !readiness.ready) pollTimer = setTimeout(check, startPollMs)
  }

  function onVisible() {
    if (document.visibilityState === 'visible' && readiness && !readiness.ready) void check()
  }

  onMount(() => {
    void check()
    void (async () => {
      try {
        projects = await listEasyProjects()
      } catch {
        loadFailed = true
      }
    })()
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      clearTimeout(pollTimer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  })

  function errorText(code: string): string {
    return $t.easy.errors[code] ?? $t.easy.errors.unknown
  }

  async function start() {
    if (starting) return
    errorCode = ''
    if (!idea.trim()) {
      errorCode = 'idea_required'
      return
    }
    starting = true
    try {
      const { project } = await createEasyProject(idea)
      onNavigate(easyProjectPath(project.id))
    } catch (err) {
      const code = err instanceof APIRequestError ? String((err.payload as { code?: string } | undefined)?.code ?? '') : ''
      errorCode = code || 'unknown'
      // Node went away since the last answer: show the Start screen again.
      if (code === 'node_missing') void check()
    } finally {
      starting = false
    }
  }
</script>

<div class="easy" data-testid="easy-root">
  {#if readiness && !readiness.ready}
    <EasyStart {readiness} {checking} onCheck={check} {onNavigate} />
  {:else}
  <section class="make">
    <h1>{$t.easy.home.title}</h1>
    <p class="subtitle">{$t.easy.home.subtitle}</p>
    <textarea
      data-testid="easy-idea"
      rows="3"
      placeholder={$t.easy.home.placeholder}
      bind:value={idea}
      onkeydown={(e) => {
        if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) start()
      }}
    ></textarea>
    {#if errorCode}
      <p class="error" role="alert" data-testid="easy-error">{errorText(errorCode)}</p>
    {/if}
    <button type="button" class="btn btn-primary start" data-testid="easy-start" disabled={starting} onclick={start}>
      {starting ? $t.easy.home.starting : $t.easy.home.start}
    </button>
    <div class="examples">
      <span>{$t.easy.home.examplesTitle}</span>
      {#each $t.easy.home.examples as example (example)}
        <button type="button" class="chip" onclick={() => (idea = example)}>{example}</button>
      {/each}
    </div>
  </section>
  {/if}

  <section class="projects">
    <h2>{$t.easy.home.projectsTitle}</h2>
    {#if loadFailed}
      <p class="muted">{$t.easy.home.loadFailed}</p>
    {:else if projects.length === 0}
      <p class="muted">{$t.easy.home.empty}</p>
    {:else}
      <ul>
        {#each projects as project (project.id)}
          <li>
            <button type="button" class="project" data-testid="easy-project-item" onclick={() => onNavigate(easyProjectPath(project.id))}>
              <span class="name">{project.name}</span>
              <span class="status">{project.status === 'failed' ? $t.easy.status.failed : project.status === 'creating' ? $t.easy.status.creating : ''}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if consoleMode.expert}
    <footer>
      <button type="button" class="btn btn-ghost btn-sm" data-testid="easy-expert" onclick={() => onNavigate('/console')}>{$t.easy.home.expert}</button>
    </footer>
  {/if}
</div>

<style>
  .easy {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
    max-width: 720px;
    margin: 0 auto;
    padding: var(--space-10) var(--space-6);
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    margin: 0 0 var(--space-2);
  }
  h2 {
    font-size: var(--text-lg);
    margin: 0 0 var(--space-3);
  }
  .subtitle,
  .muted {
    color: var(--text-secondary);
    margin: 0 0 var(--space-4);
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-md);
    resize: vertical;
  }
  .start {
    margin-top: var(--space-3);
    min-width: 160px;
  }
  .error {
    color: var(--error);
    margin: var(--space-2) 0 0;
  }
  .examples {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-4);
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }
  .chip {
    border: 1px solid var(--border-subtle);
    border-radius: 999px;
    background: var(--surface);
    color: var(--text-secondary);
    padding: var(--space-1) var(--space-3);
    font: inherit;
    cursor: pointer;
  }
  .chip:hover {
    background: var(--surface-hover);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .project {
    display: flex;
    justify-content: space-between;
    width: 100%;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .project:hover {
    background: var(--surface-hover);
  }
  .status {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }
  footer {
    display: flex;
    justify-content: flex-end;
  }
</style>
