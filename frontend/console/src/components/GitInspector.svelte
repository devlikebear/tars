<script lang="ts">
  import { onMount } from 'svelte'
  import { locale, t } from '../i18n'
  import { createGitMutationApproval, getGitBranches, getGitCommit, getGitDiff, getGitLog, getGitStatus, getGitWorktrees, type CreateGitMutationApprovalRequest } from '../lib/api'
  import type { GitBranch, GitBranchesResponse, GitCommit, GitCommitDetail, GitCommitFile, GitDiff, GitStatus, GitStatusFile, GitWorktree } from '../lib/types'
  import { parseUnifiedDiff } from '../lib/diff'
  import DiffView from './DiffView.svelte'

  interface Props {
    sessionId: string
    onClose?: () => void
  }

  let { sessionId, onClose }: Props = $props()

  type TabId = 'status' | 'files' | 'branches' | 'log' | 'worktrees'
  type DiffMode = 'unified' | 'split'
  type DiffSource =
    | { kind: 'workdir'; path: string; staged: boolean }
    | { kind: 'commit'; hash: string; path: string }

  let status = $state<GitStatus | null>(null)
  let diff = $state<GitDiff | null>(null)
  let log = $state<GitCommit[]>([])
  let branches = $state<GitBranch[]>([])
  let worktrees = $state<GitWorktree[]>([])
  let worktreesLoaded = $state(false)
  let worktreesLoading = $state(false)
  let commitDetails = $state<Record<string, GitCommitDetail>>({})
  let commitLoading = $state<Record<string, boolean>>({})
  let expandedCommit = $state('')
  let diffSource = $state<DiffSource | null>(null)
  let loading = $state(false)
  let diffLoading = $state(false)
  let error = $state('')
  let mutationBusy = $state('')
  let mutationFeedback = $state('')
  let commitMessage = $state('')
  let activeTab = $state<TabId>('files')
  let diffMode = $state<DiffMode>('unified')
  let checkoutBranch = $state<Record<string, string>>({})
  let newWorktreePath = $state('')
  let newWorktreeBranch = $state('')
  let newWorktreeNewBranch = $state('')

  let files = $derived.by<GitStatusFile[]>(() => status?.files ?? [])
  let remotes = $derived.by(() => status?.remotes ?? [])
  let changedCount = $derived(files.length)
  let stagedCount = $derived(files.filter((file) => file.staged).length)
  let unstagedCount = $derived(files.filter((file) => file.unstaged).length)
  let currentBranch = $derived.by<GitBranch | undefined>(() => branches.find((branch) => branch.current))
  let diffLines = $derived(parseUnifiedDiff(diff?.patch))
  let selectedPath = $derived(diffSource?.path ?? '')
  let selectedStaged = $derived(diffSource?.kind === 'workdir' && diffSource.staged === true)
  let activeWorkdirPath = $derived(diffSource?.kind === 'workdir' ? diffSource.path : '')
  let activeCommitFile = $derived(diffSource?.kind === 'commit' ? `${diffSource.hash}:${diffSource.path}` : '')

  function shortPath(path: string, max = 40): string {
    if (path.length <= max) return path
    return `…${path.slice(-(max - 1))}`
  }

  function formatDate(value?: string): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString($locale, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }

  function fileTone(file: GitStatusFile): string {
    if (file.untracked) return 'info'
    if (file.status === 'deleted') return 'error'
    if (file.staged) return 'success'
    return 'warning'
  }

  function branchLabel(branch?: GitBranch): string {
    if (!branch) return status?.branch || $t.gitInspector.detached
    return branch.name
  }

  // The server sends a closed set of status words; a new one shows as sent.
  function fileStatusLabel(value: string): string {
    const labels: Record<string, string> = $t.gitInspector.fileStatus
    return labels[value] ?? value
  }

  async function load() {
    loading = true
    error = ''
    diff = null
    diffSource = null
    commitDetails = {}
    expandedCommit = ''
    worktreesLoaded = false
    try {
      const nextStatus = await getGitStatus({ sessionId })
      status = nextStatus
      if (!nextStatus.is_git) {
        log = []
        branches = []
        worktrees = []
        return
      }
      const query = { sessionId, root: nextStatus.root }
      const [nextLog, nextBranches] = await Promise.all([
        getGitLog({ ...query, limit: 12 }),
        getGitBranches(query),
      ])
      log = nextLog.commits ?? []
      branches = (nextBranches as GitBranchesResponse).branches ?? []
      const first = nextStatus.files?.[0]
      if (first) {
        await loadDiff(first, first.staged && !first.unstaged)
      }
      if (activeTab === 'worktrees') {
        await loadWorktrees(nextStatus.root)
      }
    } catch (err) {
      error = err instanceof Error ? err.message : $t.gitInspector.errors.loadStatus
    } finally {
      loading = false
    }
  }

  async function loadDiff(file: GitStatusFile, staged: boolean) {
    if (!status?.is_git) return
    diffSource = { kind: 'workdir', path: file.path, staged }
    diffLoading = true
    error = ''
    try {
      diff = await getGitDiff({ sessionId, root: status.root, path: file.path, staged })
    } catch (err) {
      error = err instanceof Error ? err.message : $t.gitInspector.errors.loadDiff
      diff = null
    } finally {
      diffLoading = false
    }
  }

  async function loadCommitDiff(hash: string, path: string) {
    if (!status?.is_git) return
    diffSource = { kind: 'commit', hash, path }
    activeTab = 'files'
    diffLoading = true
    error = ''
    try {
      diff = await getGitDiff({ sessionId, root: status.root, path, hash })
    } catch (err) {
      error = err instanceof Error ? err.message : $t.gitInspector.errors.loadCommitDiff
      diff = null
    } finally {
      diffLoading = false
    }
  }

  async function toggleCommit(hash: string) {
    if (!status?.is_git) return
    if (expandedCommit === hash) {
      expandedCommit = ''
      return
    }
    expandedCommit = hash
    if (commitDetails[hash] || commitLoading[hash]) return
    commitLoading = { ...commitLoading, [hash]: true }
    try {
      const detail = await getGitCommit({ sessionId, root: status.root, hash })
      commitDetails = { ...commitDetails, [hash]: detail }
    } catch (err) {
      error = err instanceof Error ? err.message : $t.gitInspector.errors.loadCommitDetail
    } finally {
      commitLoading = { ...commitLoading, [hash]: false }
    }
  }

  async function loadWorktrees(root?: string) {
    if (!status?.is_git && !root) return
    worktreesLoading = true
    error = ''
    try {
      const res = await getGitWorktrees({ sessionId, root: root ?? status?.root })
      worktrees = res.worktrees ?? []
      worktreesLoaded = true
    } catch (err) {
      error = err instanceof Error ? err.message : $t.gitInspector.errors.loadWorktrees
    } finally {
      worktreesLoading = false
    }
  }

  function selectTab(tab: TabId) {
    activeTab = tab
    if (tab === 'worktrees' && !worktreesLoaded && !worktreesLoading) {
      void loadWorktrees()
    }
  }

  async function requestMutation(action: CreateGitMutationApprovalRequest['action'], options: Partial<CreateGitMutationApprovalRequest> = {}) {
    if (!status?.is_git || mutationBusy) return
    error = ''
    mutationFeedback = ''
    const key = `${action}:${options.hash ?? options.worktree_path ?? options.path ?? options.branch ?? ''}`
    mutationBusy = key
    try {
      const plan = await createGitMutationApproval({
        session_id: sessionId,
        root: status.root,
        action,
        ...options,
      })
      mutationFeedback = $t.gitInspector.feedback.queuedForApproval(plan.approval_id)
    } catch (err) {
      error = err instanceof Error ? err.message : $t.gitInspector.errors.queueMutation
    } finally {
      mutationBusy = ''
    }
  }

  async function requestCommit() {
    const message = commitMessage.trim()
    if (!message) {
      error = $t.gitInspector.errors.commitMessageRequired
      return
    }
    await requestMutation('commit', { message, reason: 'Commit staged changes from Git Inspector' })
  }

  async function requestFetch() {
    await requestMutation('fetch', { reason: 'Fetch all remotes from Git Inspector' })
  }

  function stripRemotePrefix(name: string): string {
    for (const remote of remotes) {
      if (name.startsWith(remote.name + '/')) {
        return name.slice(remote.name.length + 1)
      }
    }
    const idx = name.indexOf('/')
    return idx >= 0 ? name.slice(idx + 1) : name
  }

  async function requestCheckoutCommit(hash: string) {
    const newBranch = (checkoutBranch[hash] ?? '').trim()
    const reason = newBranch
      ? `Checkout commit ${hash.slice(0, 7)} as new branch ${newBranch}`
      : `Checkout commit ${hash.slice(0, 7)} (detached HEAD)`
    await requestMutation('checkout_commit', {
      hash,
      new_branch: newBranch || undefined,
      reason,
    })
  }

  async function requestWorktreeAdd() {
    const path = newWorktreePath.trim()
    const branch = newWorktreeBranch.trim()
    const newBranch = newWorktreeNewBranch.trim()
    if (!path) {
      error = $t.gitInspector.errors.worktreePathRequired
      return
    }
    await requestMutation('worktree_add', {
      worktree_path: path,
      branch: branch || undefined,
      new_branch: newBranch || undefined,
      reason: `Add worktree at ${path}`,
    })
    if (!error) {
      newWorktreePath = ''
      newWorktreeBranch = ''
      newWorktreeNewBranch = ''
    }
  }

  async function requestWorktreeRemove(path: string) {
    await requestMutation('worktree_remove', {
      worktree_path: path,
      reason: `Remove worktree at ${path}`,
    })
  }

  function tabLabel(id: TabId): string {
    return $t.gitInspector.tabs[id]
  }

  function tabBadge(id: TabId): string {
    switch (id) {
      case 'files':
        return changedCount > 0 ? String(changedCount) : ''
      case 'branches':
        return branches.length > 0 ? String(branches.filter((b) => !b.remote).length) : ''
      case 'log':
        return log.length > 0 ? String(log.length) : ''
      case 'worktrees':
        return worktrees.length > 1 ? String(worktrees.length) : ''
      default:
        return ''
    }
  }

  function commitFileTone(file: GitCommitFile): string {
    switch (file.status) {
      case 'added':
        return 'success'
      case 'deleted':
        return 'error'
      case 'renamed':
      case 'copied':
        return 'info'
      default:
        return 'warning'
    }
  }

  onMount(() => {
    void load()
  })
</script>

<div class="git-panel">
  <header class="git-header">
    <div class="git-heading">
      <span class="panel-kicker">Git</span>
      <strong class="branch-name" title={branchLabel(currentBranch)}>{branchLabel(currentBranch)}</strong>
      {#if status?.head}
        <span class="meta-chip" title="HEAD">{status.head}</span>
      {/if}
      {#if status?.upstream}
        <span class="meta-chip subtle" title={$t.gitInspector.header.upstreamTitle}>↑ {status.upstream}</span>
      {/if}
      {#if status?.is_git}
        <span class="meta-chip subtle" title={$t.gitInspector.header.countsTitle}>
          Δ{changedCount} · S{stagedCount} · U{unstagedCount}
        </span>
      {/if}
    </div>
    <div class="git-actions">
      <button class="btn btn-ghost btn-sm" type="button" disabled={loading} onclick={load}>{$t.common.actions.refresh}</button>
      {#if onClose}
        <button class="btn btn-ghost btn-sm" type="button" onclick={onClose}>{$t.common.actions.close}</button>
      {/if}
    </div>
  </header>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}
  {#if mutationFeedback}
    <div class="success-banner">{mutationFeedback}</div>
  {/if}

  {#if loading && !status}
    <div class="empty-state">{$t.gitInspector.loadingState}</div>
  {:else if status && !status.is_git}
    <div class="empty-state">{$t.gitInspector.notRepository}</div>
  {:else if status}
    <div class="tab-nav" role="tablist" aria-label={$t.gitInspector.tabsLabel}>
      {#each ['status', 'files', 'branches', 'log', 'worktrees'] as const as tab (tab)}
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === tab}
          class="tab-btn"
          class:active={activeTab === tab}
          onclick={() => selectTab(tab)}
        >
          <span>{tabLabel(tab)}</span>
          {#if tabBadge(tab)}
            <span class="tab-badge badge badge-default">{tabBadge(tab)}</span>
          {/if}
        </button>
      {/each}
    </div>

    {#if activeTab === 'status'}
      <section class="tab-body" role="tabpanel">
        <dl class="status-meta">
          <div>
            <dt>{$t.gitInspector.status.root}</dt>
            <dd title={status.root}>{status.root}</dd>
          </div>
          <div>
            <dt>HEAD</dt>
            <dd>{status.head || $t.gitInspector.status.unborn}</dd>
          </div>
          <div>
            <dt>{$t.gitInspector.status.branch}</dt>
            <dd>{status.branch || $t.gitInspector.detached}</dd>
          </div>
          {#if status.upstream}
            <div>
              <dt>{$t.gitInspector.status.upstream}</dt>
              <dd>{status.upstream}</dd>
            </div>
          {/if}
          <div>
            <dt>{$t.gitInspector.status.changed}</dt>
            <dd>{changedCount}</dd>
          </div>
          <div>
            <dt>{$t.gitInspector.status.staged}</dt>
            <dd>{stagedCount}</dd>
          </div>
          <div>
            <dt>{$t.gitInspector.status.unstaged}</dt>
            <dd>{unstagedCount}</dd>
          </div>
        </dl>

        {#if remotes.length > 0}
          <div class="section-title">{$t.gitInspector.status.remotes}</div>
          <div class="remote-list">
            {#each remotes as remote (remote.name)}
              <article class="remote-row">
                <strong>{remote.name}</strong>
                <span title={remote.fetch_url || remote.push_url}>{remote.fetch_url || remote.push_url}</span>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {:else if activeTab === 'files'}
      <section class="tab-body" role="tabpanel">
        {#if files.length === 0}
          <div class="empty-state compact">{$t.gitInspector.files.clean}</div>
        {:else}
          <div class="file-list" role="list">
            {#each files as file (file.path)}
              <article class="file-row" class:active={activeWorkdirPath === file.path} role="listitem">
                <button type="button" class="file-main" onclick={() => loadDiff(file, file.staged && !file.unstaged)}>
                  <span class="file-path" title={file.path}>{shortPath(file.path)}</span>
                  <small>{file.old_path ? `${file.old_path} → ${file.path}` : fileStatusLabel(file.status)}</small>
                </button>
                <span class="badge badge-{fileTone(file)}">{fileStatusLabel(file.status)}</span>
                <div class="file-actions">
                  {#if file.unstaged}
                    <button class="btn btn-ghost btn-sm" type="button" class:active={activeWorkdirPath === file.path && !selectedStaged} onclick={() => loadDiff(file, false)}>{$t.gitInspector.files.worktree}</button>
                    <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('stage', { path: file.path, reason: 'Stage selected file from Git Inspector' })}>
                      {mutationBusy === `stage:${file.path}` ? '…' : $t.gitInspector.files.stage}
                    </button>
                    <button class="btn btn-danger btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('discard', { path: file.path, reason: 'Discard selected worktree changes from Git Inspector' })}>
                      {mutationBusy === `discard:${file.path}` ? '…' : $t.gitInspector.files.discard}
                    </button>
                  {/if}
                  {#if file.staged}
                    <button class="btn btn-ghost btn-sm" type="button" class:active={activeWorkdirPath === file.path && selectedStaged} onclick={() => loadDiff(file, true)}>{$t.gitInspector.files.staged}</button>
                    <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('unstage', { path: file.path, reason: 'Unstage selected file from Git Inspector' })}>
                      {mutationBusy === `unstage:${file.path}` ? '…' : $t.gitInspector.files.unstage}
                    </button>
                  {/if}
                </div>
              </article>
            {/each}
          </div>
        {/if}

        {#if diff || diffLoading || diffSource?.kind === 'commit'}
          <div class="diff-section">
            <div class="diff-head">
              <div class="diff-head-title">
                <span class="section-title">{$t.gitInspector.diff.title}</span>
                {#if diff?.path}
                  <strong title={diff.path}>{shortPath(diff.path, 60)}</strong>
                  {#if diff.hash}
                    <span class="badge badge-info" title={diff.hash}>{diff.hash.slice(0, 7)}</span>
                  {:else}
                    <span class="badge badge-default">{diff.staged ? $t.gitInspector.diff.staged : $t.gitInspector.diff.worktree}</span>
                  {/if}
                {/if}
              </div>
              {#if diff}
                <div class="diff-mode" role="group" aria-label={$t.gitInspector.diff.layoutLabel}>
                  <button type="button" class="btn btn-ghost btn-sm" class:active={diffMode === 'unified'} onclick={() => (diffMode = 'unified')}>{$t.gitInspector.diff.unified}</button>
                  <button type="button" class="btn btn-ghost btn-sm" class:active={diffMode === 'split'} onclick={() => (diffMode = 'split')}>{$t.gitInspector.diff.split}</button>
                </div>
              {/if}
            </div>
            {#if diffLoading}
              <div class="empty-state compact">{$t.gitInspector.diff.loading}</div>
            {:else if !diff}
              <div class="empty-state compact">{$t.gitInspector.diff.selectFile}</div>
            {:else if diffLines.length === 0}
              <div class="empty-state compact">{$t.gitInspector.diff.empty}</div>
            {:else}
              <DiffView lines={diffLines} mode={diffMode} label={$t.gitInspector.diff.sideBySideLabel} />
            {/if}
          </div>
        {/if}
      </section>
    {:else if activeTab === 'branches'}
      <section class="tab-body" role="tabpanel">
        <div class="branches-head">
          <span class="section-title">{$t.gitInspector.tabs.branches}</span>
          <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={requestFetch}>
            {mutationBusy === 'fetch:' ? '…' : $t.gitInspector.branches.fetch}
          </button>
        </div>
        <div class="commit-box">
          <input type="text" bind:value={commitMessage} placeholder={$t.gitInspector.branches.commitPlaceholder} />
          <button class="btn btn-primary btn-sm" type="button" disabled={stagedCount === 0 || !commitMessage.trim() || !!mutationBusy} onclick={requestCommit}>
            {mutationBusy === 'commit:' ? $t.gitInspector.queueing : $t.gitInspector.branches.commitStaged(stagedCount)}
          </button>
        </div>
        {#if branches.length === 0}
          <div class="empty-state compact">{$t.gitInspector.branches.empty}</div>
        {:else}
          <div class="branch-list">
            {#each branches as branch (branch.name)}
              {@const shortName = branch.remote ? stripRemotePrefix(branch.name) : branch.name}
              <article class="branch-row" class:current={branch.current}>
                <div class="branch-meta">
                  <strong title={branch.name}>
                    {#if branch.current}<span class="branch-marker" aria-label={$t.gitInspector.current}>●</span>{/if}
                    {shortName}
                  </strong>
                  {#if branch.remote}
                    <small class="branch-remote">{branch.name}</small>
                  {:else if branch.upstream}
                    <small>↑ {branch.upstream}</small>
                  {/if}
                </div>
                <div class="branch-actions">
                  {#if branch.current}
                    <span class="badge badge-default">{$t.gitInspector.current}</span>
                  {:else if branch.remote}
                    <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('switch_branch', { branch: shortName, reason: `Checkout remote branch ${branch.name}` })}>
                      {mutationBusy === `switch_branch:${shortName}` ? '…' : $t.gitInspector.branches.checkout}
                    </button>
                  {:else}
                    <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestMutation('switch_branch', { branch: branch.name, reason: 'Switch branch from Git Inspector' })}>
                      {mutationBusy === `switch_branch:${branch.name}` ? '…' : $t.gitInspector.branches.switch}
                    </button>
                  {/if}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {:else if activeTab === 'log'}
      <section class="tab-body" role="tabpanel">
        {#if log.length === 0}
          <div class="empty-state compact">{$t.gitInspector.log.empty}</div>
        {:else}
          <div class="log-list">
            {#each log as commit (commit.hash)}
              {@const expanded = expandedCommit === commit.hash}
              {@const detail = commitDetails[commit.hash]}
              {@const detailLoading = commitLoading[commit.hash]}
              <article class="log-row" class:expanded>
                <button class="log-row-button" type="button" onclick={() => toggleCommit(commit.hash)} aria-expanded={expanded}>
                  <div class="log-row-head">
                    <strong title={commit.hash}>{commit.short_hash}</strong>
                    <small>{formatDate(commit.date)}</small>
                  </div>
                  <span class="log-subject">{commit.subject}</span>
                  {#if commit.author}
                    <small class="log-author">{commit.author}</small>
                  {/if}
                </button>
                {#if expanded}
                  <div class="log-detail">
                    {#if detailLoading}
                      <div class="empty-state compact">{$t.gitInspector.log.loadingCommit}</div>
                    {:else if !detail}
                      <div class="empty-state compact">{$t.gitInspector.log.noDetail}</div>
                    {:else}
                      {#if detail.body}
                        <pre class="commit-body">{detail.body}</pre>
                      {/if}
                      {#if detail.files.length === 0}
                        <div class="empty-state compact">{$t.gitInspector.log.noFileChanges}</div>
                      {:else}
                        <div class="commit-file-list">
                          {#each detail.files as cf (cf.path)}
                            {@const isActive = activeCommitFile === `${commit.hash}:${cf.path}`}
                            <button type="button" class="commit-file-row" class:active={isActive} onclick={() => loadCommitDiff(commit.hash, cf.path)}>
                              <span class="commit-file-path" title={cf.old_path ? `${cf.old_path} → ${cf.path}` : cf.path}>
                                {shortPath(cf.path)}
                              </span>
                              <span class="badge badge-{commitFileTone(cf)}">{fileStatusLabel(cf.status)}</span>
                              {#if cf.binary}
                                <small class="commit-file-stat">{$t.gitInspector.log.binary}</small>
                              {:else}
                                <small class="commit-file-stat">+{cf.additions} −{cf.deletions}</small>
                              {/if}
                            </button>
                          {/each}
                        </div>
                      {/if}
                      <div class="commit-actions">
                        <input
                          type="text"
                          placeholder={$t.gitInspector.log.newBranchPlaceholder}
                          value={checkoutBranch[commit.hash] ?? ''}
                          oninput={(e) => (checkoutBranch = { ...checkoutBranch, [commit.hash]: (e.currentTarget as HTMLInputElement).value })}
                        />
                        {#if (checkoutBranch[commit.hash] ?? '').trim()}
                          <button class="btn btn-ghost btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestCheckoutCommit(commit.hash)}>
                            {mutationBusy === `checkout_commit:${commit.hash}` ? '…' : $t.gitInspector.log.checkoutAsBranch}
                          </button>
                        {:else}
                          <button class="btn btn-danger btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestCheckoutCommit(commit.hash)} title={$t.gitInspector.log.detachedWarning}>
                            {mutationBusy === `checkout_commit:${commit.hash}` ? '…' : $t.gitInspector.log.checkoutDetached}
                          </button>
                        {/if}
                      </div>
                    {/if}
                  </div>
                {/if}
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {:else if activeTab === 'worktrees'}
      <section class="tab-body" role="tabpanel">
        <div class="worktrees-head">
          <span class="section-title">{$t.gitInspector.tabs.worktrees}</span>
          <button class="btn btn-ghost btn-sm" type="button" disabled={worktreesLoading} onclick={() => loadWorktrees()}>
            {worktreesLoading ? $t.gitInspector.worktrees.loading : $t.common.actions.refresh}
          </button>
        </div>

        <form class="worktree-add" onsubmit={(e) => { e.preventDefault(); void requestWorktreeAdd() }}>
          <div class="section-title">{$t.gitInspector.worktrees.addTitle}</div>
          <input type="text" bind:value={newWorktreePath} placeholder={$t.gitInspector.worktrees.pathPlaceholder} />
          <div class="worktree-add-row">
            <input type="text" bind:value={newWorktreeBranch} placeholder={$t.gitInspector.worktrees.branchPlaceholder} />
            <input type="text" bind:value={newWorktreeNewBranch} placeholder={$t.gitInspector.worktrees.newBranchPlaceholder} />
          </div>
          <button class="btn btn-primary btn-sm" type="submit" disabled={!newWorktreePath.trim() || !!mutationBusy}>
            {mutationBusy.startsWith('worktree_add:') ? $t.gitInspector.queueing : $t.gitInspector.worktrees.queueAdd}
          </button>
        </form>

        {#if worktreesLoading && worktrees.length === 0}
          <div class="empty-state compact">{$t.gitInspector.worktrees.loadingList}</div>
        {:else if worktrees.length === 0}
          <div class="empty-state compact">{$t.gitInspector.worktrees.empty}</div>
        {:else}
          <div class="worktree-list">
            {#each worktrees as wt (wt.path)}
              <article class="worktree-row" class:current={wt.current}>
                <div class="worktree-meta">
                  <strong title={wt.path}>
                    {#if wt.current}<span class="branch-marker" aria-label={$t.gitInspector.current}>●</span>{/if}
                    {wt.branch || (wt.detached ? $t.gitInspector.detached : wt.bare ? $t.gitInspector.worktrees.bareLabel : $t.gitInspector.worktrees.unknownLabel)}
                  </strong>
                  <small class="worktree-path" title={wt.path}>{wt.path}</small>
                  <div class="worktree-tags">
                    {#if wt.head}<span class="meta-chip" title={wt.head}>{wt.head.slice(0, 7)}</span>{/if}
                    {#if wt.detached}<span class="badge badge-warning">{$t.gitInspector.worktrees.badges.detached}</span>{/if}
                    {#if wt.locked}<span class="badge badge-info" title={wt.lock_reason}>{$t.gitInspector.worktrees.badges.locked}</span>{/if}
                    {#if wt.prunable}<span class="badge badge-error" title={wt.prune_reason}>{$t.gitInspector.worktrees.badges.prunable}</span>{/if}
                    {#if wt.bare}<span class="badge badge-default">{$t.gitInspector.worktrees.badges.bare}</span>{/if}
                  </div>
                </div>
                <div class="worktree-actions">
                  {#if !wt.current && !wt.bare}
                    <button class="btn btn-danger btn-sm" type="button" disabled={!!mutationBusy} onclick={() => requestWorktreeRemove(wt.path)}>
                      {mutationBusy === `worktree_remove:${wt.path}` ? '…' : $t.gitInspector.worktrees.remove}
                    </button>
                  {/if}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .git-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
    min-height: 0;
    height: 100%;
  }

  .git-header,
  .error-banner,
  .success-banner,
  .tab-nav {
    flex: 0 0 auto;
  }

  .git-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .git-heading {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    flex-wrap: wrap;
    min-width: 0;
  }

  .git-actions {
    display: flex;
    gap: var(--space-1);
    align-items: center;
  }

  .panel-kicker,
  .section-title,
  .status-meta dt {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .branch-name {
    color: var(--text-primary);
    font-size: var(--text-md);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: min(100%, 240px);
  }

  .meta-chip {
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: 2px var(--space-1);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    white-space: nowrap;
  }

  .meta-chip.subtle {
    border-color: transparent;
    background: transparent;
    padding-left: 0;
    padding-right: 0;
  }

  .tab-nav {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--border-subtle);
    padding-bottom: var(--space-1);
    flex-wrap: wrap;
  }

  .tab-btn {
    background: none;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    padding: var(--space-1) var(--space-2);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .tab-btn:hover {
    color: var(--text-primary);
    background: var(--surface-hover);
  }

  .tab-btn.active {
    color: var(--text-primary);
    background: var(--surface-elevated);
    border-color: var(--border-default);
  }

  .tab-badge {
    font-size: var(--text-xs);
    padding: 1px 5px;
  }

  .tab-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
    min-height: 0;
    flex: 1 1 auto;
    overflow-y: auto;
    padding-right: var(--space-1);
  }

  .status-meta {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--space-2);
    margin: 0;
  }

  .status-meta div {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
    min-width: 0;
    display: grid;
    gap: 2px;
  }

  .status-meta dt {
    margin: 0;
  }

  .status-meta dd {
    margin: 0;
    color: var(--text-primary);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  .remote-list {
    display: grid;
    gap: var(--space-1);
  }

  .remote-row {
    display: grid;
    grid-template-columns: minmax(80px, max-content) minmax(0, 1fr);
    gap: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
    align-items: baseline;
  }

  .remote-row strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .remote-row span {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }

  .file-list {
    display: grid;
    gap: var(--space-1);
    max-height: 220px;
    overflow-y: auto;
    padding-right: 2px;
  }

  .file-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-2);
    align-items: center;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
    min-width: 0;
  }

  .file-row.active {
    border-color: var(--primary);
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

  .file-main .file-path {
    color: var(--text-primary);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-main small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .file-actions {
    grid-column: 1 / -1;
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
  }

  .file-actions .active {
    color: var(--primary-text);
    border-color: var(--primary);
  }

  .diff-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
    min-height: 240px;
    flex: 1 1 auto;
  }

  .diff-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .diff-head-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
    min-width: 0;
  }

  .diff-head-title strong {
    color: var(--text-primary);
    font-size: var(--text-sm);
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: min(100%, 320px);
  }

  .diff-mode {
    display: inline-flex;
    gap: 2px;
  }

  .diff-mode .active {
    color: var(--primary-text);
    border-color: var(--primary);
  }

  .branch-list,
  .log-list {
    display: grid;
    gap: var(--space-1);
  }

  .branch-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
    min-width: 0;
  }

  .branch-row.current {
    border-color: var(--primary);
  }

  .branch-meta {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .branch-meta strong {
    color: var(--text-primary);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .branch-marker {
    color: var(--primary);
    font-size: 10px;
  }

  .branch-meta small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .branch-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .commit-box {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .commit-box input {
    flex: 1;
    min-width: 160px;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-sm);
  }

  .log-row {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    display: grid;
    min-width: 0;
    overflow: hidden;
  }

  .log-row.expanded {
    border-color: var(--primary);
  }

  .log-row-button {
    display: grid;
    gap: 2px;
    border: 0;
    background: transparent;
    color: inherit;
    text-align: left;
    padding: var(--space-2);
    cursor: pointer;
    min-width: 0;
  }

  .log-row-button:hover {
    background: var(--surface-hover);
  }

  .log-detail {
    border-top: 1px solid var(--border-subtle);
    padding: var(--space-2);
    display: grid;
    gap: var(--space-2);
    background: var(--surface-inset);
  }

  .commit-body {
    margin: 0;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .commit-file-list {
    display: grid;
    gap: var(--space-1);
  }

  .commit-file-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) max-content max-content;
    gap: var(--space-2);
    align-items: center;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-1) var(--space-2);
    cursor: pointer;
    color: inherit;
    text-align: left;
    min-width: 0;
  }

  .commit-file-row:hover {
    background: var(--surface-hover);
  }

  .commit-file-row.active {
    border-color: var(--primary);
    background: var(--surface-elevated);
  }

  .commit-file-path {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .commit-file-stat {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .worktrees-head,
  .branches-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-2);
  }

  .branch-remote {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .worktree-list {
    display: grid;
    gap: var(--space-1);
  }

  .worktree-row {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
    min-width: 0;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .worktree-row.current {
    border-color: var(--primary);
  }

  .worktree-meta {
    display: grid;
    gap: 2px;
    min-width: 0;
    flex: 1;
  }

  .worktree-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .worktree-add {
    display: grid;
    gap: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
  }

  .worktree-add input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-sm);
    min-width: 0;
  }

  .worktree-add-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-2);
  }

  .commit-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    align-items: center;
    margin-top: var(--space-1);
  }

  .commit-actions input {
    flex: 1;
    min-width: 160px;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-sm);
  }

  .worktree-meta strong {
    color: var(--text-primary);
    font-size: var(--text-sm);
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .worktree-path {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    overflow-wrap: anywhere;
  }

  .worktree-tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    align-items: center;
    margin-top: 2px;
  }

  .log-row-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: var(--space-2);
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .log-row-head strong {
    font-family: var(--font-mono);
  }

  .log-row-head small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .log-subject {
    color: var(--text-secondary);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  .log-author {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .compact {
    padding: var(--space-3);
  }

  @media (max-width: 600px) {
    .branch-name {
      max-width: 100%;
    }

    .worktree-add-row {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
