<script lang="ts">
  // Easy mode's home (docs/plans/vibe-mode-redesign unit 2,
  // screens/D-Home): one box to say what to make, the team that will make
  // it, ideas to start from, and the three most recent projects with one
  // waiting on the person first. Starting a project is POST
  // /v1/easy/projects; the server picks the folder, sets the stack up and
  // opens the team's work, so the next screen is the project.
  //
  // While this computer lacks what a project needs (GET /v1/easy/readiness),
  // the Start screen takes the make box's place and the answer is read again
  // every few seconds, and whenever the tab comes back into view, until it
  // is ready. A server that cannot answer leaves the make box as it was.
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import { createEasyProject, easyErrorCode, getEasyReadiness, listEasyBoard } from '../../lib/api/easy'
  import { EASY_BOTS, easyProjectPath, startPollMs, type EasyReadiness } from '../../lib/easy'
  import { boardTotal, easyProjectsPath, homeRecent, HOME_RECENT, type EasyListed } from '../../lib/easyBoard'
  import { easyReadiness } from '../../lib/stores/easyReadiness.svelte'
  import EasyProjectCard from './EasyProjectCard.svelte'
  import EasyStart from './EasyStart.svelte'

  let { onNavigate }: { onNavigate: (path: string) => void } = $props()

  let idea = $state('')
  let starting = $state(false)
  let errorCode = $state('')
  let recent = $state<EasyListed[]>([])
  let total = $state(0)
  let loadFailed = $state(false)
  let readiness = $state<EasyReadiness | null>(null)
  let checking = $state(false)
  let ideaBox = $state<HTMLTextAreaElement | null>(null)
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
    easyReadiness.ai = readiness ? (readiness.ready ? 'ready' : 'notReady') : 'unknown'
    if (readiness && !readiness.ready) pollTimer = setTimeout(check, startPollMs)
  }

  async function loadRecent() {
    try {
      const [latest, waiting] = await Promise.all([listEasyBoard({ limit: HOME_RECENT }), listEasyBoard({ state: 'your_turn', limit: 1 })])
      recent = homeRecent(latest.projects, waiting.projects[0])
      total = boardTotal(latest.counts)
      loadFailed = false
    } catch {
      loadFailed = true
    }
  }

  function onVisible() {
    if (document.visibilityState !== 'visible') return
    void loadRecent()
    if (readiness && !readiness.ready) void check()
  }

  onMount(() => {
    void check()
    void loadRecent()
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
      const code = easyErrorCode(err)
      errorCode = code || 'unknown'
      // Node went away since the last answer: show the Start screen again.
      if (code === 'node_missing') void check()
    } finally {
      starting = false
    }
  }

  function useExample(text: string) {
    idea = text
    errorCode = ''
    ideaBox?.focus()
  }

  function go(event: MouseEvent, path: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onNavigate(path)
  }
</script>

<div class="easy" data-testid="easy-root">
  {#if readiness && !readiness.ready}
    <EasyStart {readiness} {checking} onCheck={check} {onNavigate} />
  {:else}
    <section class="make">
      <h1>{$t.easy.home.title}</h1>
      <div class="idea-box">
        <label for="easy-idea">{$t.easy.home.subtitle}</label>
        <textarea
          id="easy-idea"
          data-testid="easy-idea"
          rows="2"
          placeholder={$t.easy.home.placeholder}
          bind:value={idea}
          bind:this={ideaBox}
          onkeydown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) start()
          }}
        ></textarea>
        {#if errorCode}
          <p class="error" role="alert" data-testid="easy-error">{errorText(errorCode)}</p>
        {/if}
        <div class="idea-foot">
          <span class="scope">{$t.easy.home.scope}</span>
          <button type="button" class="btn btn-primary start" data-testid="easy-start" disabled={starting} onclick={start}>
            {starting ? $t.easy.home.starting : $t.easy.home.start}
          </button>
        </div>
      </div>
      <div class="team" data-testid="easy-team">
        <span class="team-title">{$t.easy.home.teamTitle}</span>
        {#each EASY_BOTS as bot, i (bot)}
          {#if i > 0}<span class="sep" aria-hidden="true">›</span>{/if}
          <span class="bot">
            <span class="avatar" aria-hidden="true">{$t.easy.bots[bot].name.slice(0, 1)}</span>
            {$t.easy.bots[bot].name}
            <span class="role">{$t.easy.home.team[bot]}</span>
          </span>
        {/each}
      </div>
    </section>

    <section class="examples">
      <h2 class="kicker">{$t.easy.home.examplesTitle}</h2>
      <div class="example-grid">
        {#each $t.easy.home.examples as example (example.idea)}
          <button type="button" class="example" data-testid="easy-example" onclick={() => useExample(example.idea)}>
            <span class="example-idea">{example.idea}</span>
            <span class="example-tag">{example.tag}</span>
          </button>
        {/each}
      </div>
    </section>
  {/if}

  <section class="recent">
    <div class="recent-head">
      <h2 class="kicker">{$t.easy.home.projectsTitle}</h2>
      {#if total > 0}
        <a href={easyProjectsPath} class="see-all" data-testid="easy-see-all" onclick={(e) => go(e, easyProjectsPath)}>{$t.easy.home.seeAll(total)}</a>
      {/if}
    </div>
    {#if loadFailed}
      <p class="muted">{$t.easy.home.loadFailed}</p>
    {:else if recent.length === 0}
      <p class="muted" data-testid="easy-recent-empty">{$t.easy.home.empty}</p>
    {:else}
      <div class="recent-grid" data-testid="easy-recent">
        {#each recent as project (project.id)}
          <EasyProjectCard {project} variant="home" {onNavigate} />
        {/each}
      </div>
    {/if}
  </section>
</div>

<style>
  .easy {
    display: flex;
    flex-direction: column;
    gap: 30px;
    max-width: 960px;
    margin: 0 auto;
    padding: 44px var(--space-6) 28px;
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    font-weight: 600;
    margin: 0 0 var(--space-4);
  }
  .kicker {
    margin: 0;
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.08em;
    color: var(--text-secondary);
  }
  .idea-box {
    background: var(--surface);
    border: 1px solid var(--primary);
    border-radius: var(--radius-lg);
    box-shadow: 0 0 0 4px rgba(var(--primary-rgb), 0.08);
    padding: 18px var(--space-5) 14px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .idea-box label {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  textarea {
    border: 0;
    outline: none;
    resize: none;
    font: inherit;
    font-size: var(--text-lg);
    line-height: 1.5;
    background: transparent;
    color: var(--text-primary);
    padding: 0;
  }
  textarea::placeholder {
    color: var(--text-tertiary);
  }
  .idea-foot {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-3);
  }
  .scope {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  .start {
    margin-left: auto;
    font-size: 15px;
    font-weight: 600;
    min-height: 44px;
    padding: 0 var(--space-5);
  }
  .error {
    color: var(--error);
    margin: 0;
  }
  .team {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-4);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .team-title {
    margin-right: var(--space-1);
  }
  .bot {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 4px 10px 4px 5px;
    color: var(--text-primary);
  }
  .avatar {
    width: 20px;
    height: 20px;
    border-radius: var(--radius-md);
    background: var(--surface-active);
    display: grid;
    place-items: center;
    font-size: 11px;
    font-weight: 600;
  }
  .role {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .sep {
    color: var(--text-ghost);
  }
  .examples,
  .recent {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .example-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-3);
  }
  .example {
    font: inherit;
    color: var(--text-primary);
    text-align: left;
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-height: 76px;
    cursor: pointer;
  }
  .example:hover {
    border-color: var(--border-strong);
    background: var(--surface-elevated);
  }
  .example-idea {
    font-size: var(--text-base);
    font-weight: 500;
  }
  .example-tag {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  .recent-head {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .see-all {
    margin-left: auto;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    text-decoration: none;
  }
  .see-all:hover {
    color: var(--text-primary);
  }
  .recent-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-3);
  }
  .muted {
    color: var(--text-secondary);
    margin: 0;
  }
  @media (max-width: 900px) {
    .example-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .recent-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 520px) {
    .easy {
      padding: var(--space-6) var(--space-4);
    }
    .example-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
