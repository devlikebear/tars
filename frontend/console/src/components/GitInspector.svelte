<script lang="ts">
  import { onMount } from 'svelte'
  import { createGitMutationApproval, getGitBranches, getGitDiff, getGitLog, getGitStatus, type CreateGitMutationApprovalRequest } from '../lib/api'
  import type { GitBranch, GitBranchesResponse, GitCommit, GitDiff, GitStatus, GitStatusFile } from '../lib/types'

  interface Props {
    sessionId: string
    onClose?: () => void
  }

  let { sessionId, onClose }: Props = $props()

  let status = $state<GitStatus | null>(null)
  let diff = $state<GitDiff | null>(null)
  let log = $state<GitCommit[]>([])
  let branches = $state<GitBranch[]>([])
  let selectedPath = $state('')
  let selectedStaged = $state(false)
  let loading = $state(false)
  let diffLoading = $state(false)
  let error = $state('')
  let mutationBusy = $state('')
  let mutationFeedback = $state('')
  let commitMessage = $state('')

  let files = $derived.by<GitStatusFile[]>(() => status?.files ?? [])
  let remotes = $derived.by(() => status?.remotes ?? [])
  let changedCount = $derived(files.length)
  let stagedCount = $derived(files.filter((file) => file.staged).length)
  let unstagedCount = $derived(files.filter((file) => file.unstaged).length)
  let currentBranch = $derived.by<GitBranch | undefined>(() => branches.find((branch) => branch.current))
  let sideBySide = $derived.by(() => sideBySideLines(diff?.patch))

  function shortPath(path: string): string {
    return path.length > 48 ? `...${path.slice(-45)}` : path
  }

  function formatDate(value?: string): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }

  function fileTone(file: GitStatusFile): string {
    if (file.untracked) return 'info'
    if (file.status === 'deleted') return 'error'
    if (file.staged) return 'success'
    return 'warning'
  }

  function branchLabel(branch?: GitBranch): string {
    if (!branch) return status?.branch || '(detached)'
    return branch.name
  }

  async function load() {
    loading = true
    error = ''
    diff = null
    try {
      const nextStatus = await getGitStatus({ sessionId })
      status = nextStatus
      if (!nextStatus.is_git) {
        log = []
        branches = []
        selectedPath = ''
        return
      }
      const query = { sessionId, root: nextStatus.root }
      const [nextLog, nextBranches] = await Promise.all([
        getGitLog({ ...query, limit: 8 }),
        getGitBranches(query),
      ])
      log = nextLog.commits ?? []
      branches = (nextBranches as GitBranchesResponse).branches ?? []
      const first = nextStatus.files?.[0]
      if (first) {
        await loadDiff(first, first.staged && !first.unstaged)
      } else {
        selectedPath = ''
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load git status'
    } finally {
      loading = false
    }
  }

  async function loadDiff(file: GitStatusFile, staged: boolean) {
    if (!status?.is_git) return
    selectedPath = file.path
    selectedStaged = staged
    diffLoading = true
    error = ''
    try {
      diff = await getGitDiff({ sessionId, root: status.root, path: file.path, staged })
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load diff'
      diff = null
    } finally {
      diffLoading = false
    }
  }

  async function requestMutation(action: CreateGitMutationApprovalRequest['action'], options: Partial<CreateGitMutationApprovalRequest> = {}) {
    if (!status?.is_git || mutationBusy) return
    error = ''
    mutationFeedback = ''
    const key = `${action}:${options.path ?? options.branch ?? ''}`
    mutationBusy = key
    try {
      const plan = await createGitMutationApproval({
        session_id: sessionId,
        root: status.root,
        action,
        ...options,
      })
      mutationFeedback = `${plan.approval_id} queued for approval`
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to queue git mutation approval'
    } finally {
      mutationBusy = ''
    }
  }

  async function requestCommit() {
    const message = commitMessage.trim()
    if (!message) {
      error = 'Commit message is required'
      return
    }
    await requestMutation('commit', { message, reason: 'Commit staged changes from Git Inspector' })
  }

  function sideBySideLines(patch?: string): { left: string[]; right: string[] } {
    const left: string[] = []
    const right: string[] = []
    for (const line of (patch ?? '').split('\n')) {
      if (line.startsWith('---') || line.startsWith('+++') || line.startsWith('diff --git')) {
        continue
      }
      if (line.startsWith('-')) {
        left.push(line.slice(1))
      } else if (line.startsWith('+')) {
        right.push(line.slice(1))
      } else if (line.startsWith('@@')) {
        left.push(line)
        right.push(line)
      }
    }
    return {
      left: left.length ? left : ['No removed lines.'],
      right: right.length ? right : ['No added lines.'],
    }
  }

  onMount(() => {
    void load()
  })
</script>

<div class="git-panel">
  <div class="git-toolbar">
    <div>
      <span class="panel-kicker">Git Inspector</span>
      <strong>{branchLabel(currentBranch)}</strong>
    </div>
    <div class="git-actions">
      <button class="btn btn-ghost btn-sm" type="button" disabled={loading} onclick={load}>Refresh</button>
      {#if onClose}
        <button class="btn btn-ghost btn-sm" type="button" onclick={onClose}>Close</button>
      {/if}
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}
  {#if mutationFeedback}
    <div class="success-banner">{mutationFeedback}</div>
  {/if}

  {#if loading && !status}
    <div class="empty-state">Loading git state...</div>
  {:else if status && !status.is_git}
    <div class="empty-state">No git repository detected for this session.</div>
  {:else if status}
    <section class="repo-summary">
      <div>
        <span>Root</span>
        <strong title={status.root}>{status.root}</strong>
      </div>
      <div>
        <span>HEAD</span>
        <strong>{status.head || 'unborn'}</strong>
      </div>
      <div>
        <span>Changed</span>
        <strong>{changedCount}</strong>
      </div>
      <div>
        <span>Staged</span>
        <strong>{stagedCount}</strong>
      </div>
      <div>
        <span>Unstaged</span>
        <strong>{unstagedCount}</strong>
      </div>
    </section>

    {#if remotes.length > 0}
      <section class="git-section">
        <div class="section-title">Remotes</div>
        <div class="remote-list">
          {#each remotes as remote (remote.name)}
            <article class="remote-row">
              <strong>{remote.name}</strong>
              <span>{remote.fetch_url || remote.push_url}</span>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    <section class="git-section">
      <div class="section-title">Files</div>
      {#if files.length === 0}
        <div class="empty-state compact">Working tree clean.</div>
      {:else}
        <div class="file-list">
          {#each files as file (file.path)}
            <article class="file-row" class:active={selectedPath === file.path}>
              <button type="button" class="file-main" onclick={() => loadDiff(file, file.staged && !file.unstaged)}>
                <span title={file.path}>{shortPath(file.path)}</span>
                <small>{file.old_path ? `${file.old_path} -> ${file.path}` : file.status}</small>
              </button>
              <span class="badge badge-{fileTone(file)}">{file.status}</span>
              <div class="file-actions">
                {#if file.unstaged}
                  <button class="btn btn-ghost btn-sm" type="button" class:active={selectedPath === file.path && !selectedStaged} onclick={() => loadDiff(file, false)}>Worktree</button>
                  <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('stage', { path: file.path, reason: 'Stage selected file from Git Inspector' })}>
                    {mutationBusy === `stage:${file.path}` ? 'Queueing...' : 'Stage'}
                  </button>
                  <button class="btn btn-danger btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('discard', { path: file.path, reason: 'Discard selected worktree changes from Git Inspector' })}>
                    {mutationBusy === `discard:${file.path}` ? 'Queueing...' : 'Discard'}
                  </button>
                {/if}
                {#if file.staged}
                  <button class="btn btn-ghost btn-sm" type="button" class:active={selectedPath === file.path && selectedStaged} onclick={() => loadDiff(file, true)}>Staged</button>
                  <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('unstage', { path: file.path, reason: 'Unstage selected file from Git Inspector' })}>
                    {mutationBusy === `unstage:${file.path}` ? 'Queueing...' : 'Unstage'}
                  </button>
                {/if}
              </div>
            </article>
          {/each}
        </div>
      {/if}
    </section>

    <section class="git-section">
      <div class="section-title">Diff</div>
      {#if diffLoading}
        <div class="empty-state compact">Loading diff...</div>
      {:else if diff}
        <div class="diff-head">
          <strong>{diff.path || 'Repository diff'}</strong>
          <span class="badge badge-default">{diff.staged ? 'staged' : 'worktree'}</span>
        </div>
        <div class="side-by-side-diff" aria-label="side-by-side diff">
          <pre>{sideBySide.left.join('\n')}</pre>
          <pre>{sideBySide.right.join('\n')}</pre>
        </div>
        <details class="raw-diff">
          <summary>Unified patch</summary>
          <pre>{diff.patch || 'No diff available.'}</pre>
        </details>
      {:else}
        <div class="empty-state compact">Select a file to inspect its diff.</div>
      {/if}
    </section>

    <section class="git-section grid-two">
      <div>
        <div class="section-title">Branches</div>
        <div class="branch-list">
          {#each branches as branch (branch.name)}
            <div class="branch-row">
              <span>{branch.current ? '*' : ''}{branch.name}</span>
              <div class="branch-actions">
                {#if branch.remote}<small>remote</small>{/if}
                {#if !branch.current && !branch.remote}
                  <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('switch_branch', { branch: branch.name, reason: 'Switch branch from Git Inspector' })}>
                    {mutationBusy === `switch_branch:${branch.name}` ? 'Queueing...' : 'Switch'}
                  </button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
      <div>
        <div class="section-title">Commit</div>
        <div class="commit-box">
          <input type="text" bind:value={commitMessage} placeholder="Commit message" />
          <button class="btn btn-primary btn-sm" type="button" disabled={stagedCount === 0 || !commitMessage.trim() || !!mutationBusy} onclick={requestCommit}>
            {mutationBusy === 'commit:' ? 'Queueing...' : 'Commit staged'}
          </button>
        </div>
        <div class="section-title">Log</div>
        <div class="log-list">
          {#each log as commit (commit.hash)}
            <article class="log-row">
              <strong>{commit.short_hash}</strong>
              <span>{commit.subject}</span>
              <small>{formatDate(commit.date)}</small>
            </article>
          {/each}
        </div>
      </div>
    </section>
  {/if}
</div>

<style>
  .git-panel {
    display: grid;
    gap: var(--space-3);
    min-width: 0;
  }

  .git-toolbar,
  .git-actions,
  .diff-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .git-toolbar strong {
    display: block;
    color: var(--text-primary);
    font-size: var(--text-md);
  }

  .panel-kicker,
  .section-title,
  .repo-summary span {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .repo-summary {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: var(--space-2);
  }

  .repo-summary div,
  .remote-row,
  .file-row,
  .log-row {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
    min-width: 0;
  }

  .repo-summary strong,
  .remote-row span,
  .file-main span,
  .file-main small,
  .branch-row span,
  .log-row span {
    overflow-wrap: anywhere;
  }

  .repo-summary strong {
    display: block;
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .git-section,
  .remote-list,
  .file-list,
  .branch-list,
  .log-list {
    display: grid;
    gap: var(--space-2);
  }

  .success-banner {
    padding: var(--space-2) var(--space-3);
    border: 1px solid rgba(34, 197, 94, 0.25);
    border-radius: var(--radius-sm);
    background: rgba(34, 197, 94, 0.08);
    color: var(--green);
    font-size: var(--text-xs);
  }

  .remote-row strong,
  .log-row strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .remote-row span,
  .log-row small,
  .branch-row small,
  .file-main small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .file-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-2);
    align-items: center;
  }

  .file-row.active {
    border-color: var(--border-default);
    background: var(--surface-elevated);
  }

  .file-main {
    display: grid;
    gap: 2px;
    min-width: 0;
    border: 0;
    background: transparent;
    color: inherit;
    text-align: left;
    padding: 0;
    cursor: pointer;
  }

  .file-main span {
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .file-actions {
    grid-column: 1 / -1;
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .file-actions .active {
    color: var(--primary-text);
    border-color: var(--primary);
  }

  .side-by-side-diff {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: var(--space-2);
  }

  .side-by-side-diff pre,
  .raw-diff pre {
    overflow: auto;
    margin: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-secondary);
    padding: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
    max-height: 260px;
  }

  .grid-two {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .branch-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .branch-actions,
  .commit-box {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .commit-box {
    margin-bottom: var(--space-3);
  }

  .commit-box input {
    min-width: 0;
    flex: 1;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-xs);
  }

  .log-row {
    display: grid;
    gap: 2px;
  }

  .log-row span {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .compact {
    padding: var(--space-3);
  }

  @media (max-width: 1100px) {
    .repo-summary,
    .grid-two,
    .side-by-side-diff {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
