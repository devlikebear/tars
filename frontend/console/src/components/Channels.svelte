<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { getTelegramPairings, approveTelegramPairing, revokeTelegramPairing } from '../lib/api'
  import type { TelegramPairingsResponse, TelegramAllowedUser, TelegramPairingEntry } from '../lib/types'
  import { t } from '../i18n'

  let pairings: TelegramPairingsResponse | null = $state(null)
  let loading = $state(true)
  let error = $state('')
  let success = $state('')
  let approveCode = $state('')
  let approving = $state(false)
  let revokeBusy = $state<number | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  async function load() {
    try {
      pairings = await getTelegramPairings()
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : $t.channels.failedLoad
    } finally {
      loading = false
    }
  }

  async function handleApprove() {
    const code = approveCode.trim()
    if (!code) {
      error = $t.channels.enterCodeError
      return
    }
    approving = true
    error = ''
    success = ''
    try {
      const result = await approveTelegramPairing(code)
      success = $t.channels.approvedSuccess(result.approved.username || String(result.approved.user_id))
      approveCode = ''
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.channels.approveFailed
    } finally {
      approving = false
    }
  }

  async function handleRevoke(userId: number) {
    revokeBusy = userId
    error = ''
    success = ''
    try {
      await revokeTelegramPairing(userId)
      success = $t.channels.accessRevoked
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.channels.revokeFailed
    } finally {
      revokeBusy = null
    }
  }

  function startPolling() {
    if (pollTimer) return
    pollTimer = setInterval(() => {
      if (!approving && revokeBusy == null) {
        load().catch(() => {})
      }
    }, 5000)
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function fmtDate(value?: string): string {
    if (!value) return $t.channels.dash
    const d = new Date(value)
    if (Number.isNaN(d.getTime())) return value
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(d)
  }

  onMount(() => {
    load()
    startPolling()
  })

  onDestroy(() => {
    stopPolling()
  })
</script>

<div class="channels-page">
  <div class="page-header">
    <h2 class="page-title">{$t.channels.title}</h2>
  </div>

  {#if error}
    <div class="message message-error">{error}</div>
  {/if}
  {#if success}
    <div class="message message-success">{success}</div>
  {/if}

  <div class="card">
    <div class="card-header">
      <span class="card-title">{$t.channels.telegramTitle}</span>
      {#if pairings}
        <div class="header-badges">
          <span class="badge badge-default">{$t.channels.policyLabel(pairings.dm_policy || $t.channels.dash)}</span>
          <span class="badge badge-default">{$t.channels.pollingLabel(pairings.polling_enabled ? $t.channels.pollingOn : $t.channels.pollingOff)}</span>
        </div>
      {/if}
    </div>

    <div class="section">
      <h3 class="section-title">{$t.channels.approveSection}</h3>
      <p class="section-desc">{$t.channels.approveDescription}</p>
      <div class="approve-row">
        <input
          type="text"
          class="field-input"
          placeholder={$t.channels.approveCodePlaceholder}
          bind:value={approveCode}
          onkeydown={(e) => { if (e.key === 'Enter') handleApprove() }}
          disabled={approving}
        />
        <button class="btn btn-primary btn-sm" disabled={approving} onclick={handleApprove}>
          {approving ? $t.channels.approving : $t.channels.approveButton}
        </button>
      </div>
    </div>

    <div class="section">
      <h3 class="section-title">
        {$t.channels.pendingSection}
        {#if pairings}
          <span class="count-badge">{pairings.pending.length}</span>
        {/if}
      </h3>
      {#if loading}
        <div class="loading">{$t.channels.loading}</div>
      {:else if !pairings || pairings.pending.length === 0}
        <div class="empty">{$t.channels.noPending}</div>
      {:else}
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{$t.channels.table.code}</th>
                <th>{$t.channels.table.user}</th>
                <th>{$t.channels.table.chatId}</th>
                <th>{$t.channels.table.expires}</th>
              </tr>
            </thead>
            <tbody>
              {#each pairings.pending as item (item.code)}
                <tr>
                  <td class="mono">{item.code}</td>
                  <td>{item.username || $t.channels.dash}</td>
                  <td class="mono">{item.chat_id}</td>
                  <td>{fmtDate(item.expires_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>

    <div class="section">
      <h3 class="section-title">
        {$t.channels.allowedSection}
        {#if pairings}
          <span class="count-badge">{pairings.allowed.length}</span>
        {/if}
      </h3>
      {#if loading}
        <div class="loading">{$t.channels.loading}</div>
      {:else if !pairings || pairings.allowed.length === 0}
        <div class="empty">{$t.channels.noAllowed}</div>
      {:else}
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>{$t.channels.table.user}</th>
                <th>{$t.channels.table.chatId}</th>
                <th>{$t.channels.table.approved}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {#each pairings.allowed as item (item.user_id)}
                <tr>
                  <td>{item.username || $t.channels.dash}</td>
                  <td class="mono">{item.chat_id}</td>
                  <td>{fmtDate(item.approved_at)}</td>
                  <td class="actions">
                    <button
                      class="btn btn-ghost btn-sm danger-hover"
                      disabled={revokeBusy === item.user_id}
                      onclick={() => handleRevoke(item.user_id)}
                    >
                      {revokeBusy === item.user_id ? $t.channels.revoking : $t.channels.revoke}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .channels-page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    overflow-y: auto;
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .page-title {
    font-family: var(--font-display);
    font-size: var(--text-xl);
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
  }

  .message {
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
  }
  .message-error {
    background: rgba(220, 60, 60, 0.15);
    color: var(--red);
    border: 1px solid rgba(220, 60, 60, 0.3);
  }
  .message-success {
    background: rgba(60, 180, 100, 0.15);
    color: var(--green);
    border: 1px solid rgba(60, 180, 100, 0.3);
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-4);
  }

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .card-title {
    font-family: var(--font-display);
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-primary);
  }

  .header-badges {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .badge {
    min-height: 18px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 1px 6px;
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
    line-height: 1.4;
    color: var(--text-tertiary);
    background: var(--surface-inset);
    white-space: nowrap;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-4);
  }

  .section-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
  }

  .section-desc {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .count-badge {
    min-height: 18px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 1px 6px;
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
    line-height: 1.4;
    color: var(--text-tertiary);
    background: var(--surface-inset);
  }

  .approve-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .field-input {
    width: 260px;
    padding: var(--space-1) var(--space-2);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    outline: none;
  }
  .field-input:focus {
    border-color: var(--primary);
    box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.3);
  }
  .field-input::placeholder {
    color: var(--text-ghost);
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-sm);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
    border: 1px solid transparent;
    white-space: nowrap;
  }
  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-primary {
    background: var(--primary);
    color: #fff;
  }
  .btn-primary:hover:not(:disabled) {
    background: var(--primary-hover, #c97a35);
  }

  .btn-ghost {
    background: transparent;
    color: var(--text-secondary);
    border-color: var(--border-subtle);
  }
  .btn-ghost:hover:not(:disabled) {
    background: var(--surface-base);
    color: var(--text-primary);
  }

  .danger-hover:hover:not(:disabled) {
    color: var(--red);
    border-color: rgba(220, 60, 60, 0.35);
    background: rgba(220, 60, 60, 0.08);
  }

  .loading {
    color: var(--text-secondary);
    font-size: var(--text-sm);
    padding: var(--space-2) 0;
  }

  .empty {
    color: var(--text-ghost);
    font-size: var(--text-sm);
    padding: var(--space-2) 0;
  }

  .table-wrap {
    overflow-x: auto;
  }

  .data-table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-xs);
  }
  .data-table th {
    text-align: left;
    padding: var(--space-2) var(--space-3);
    color: var(--text-ghost);
    font-family: var(--font-display);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    border-bottom: 1px solid var(--border-subtle);
    white-space: nowrap;
  }
  .data-table td {
    padding: var(--space-2) var(--space-3);
    color: var(--text-secondary);
    border-bottom: 1px solid var(--border-subtle);
    vertical-align: middle;
  }
  .data-table tbody tr:hover {
    background: rgba(255, 255, 255, 0.02);
  }
  .data-table .mono {
    font-family: var(--font-mono);
  }
  .data-table .actions {
    text-align: right;
    white-space: nowrap;
  }

  @media (max-width: 640px) {
    .approve-row {
      flex-direction: column;
      align-items: stretch;
    }
    .field-input {
      width: 100%;
    }
    .data-table th,
    .data-table td {
      padding: var(--space-1) var(--space-2);
    }
  }
</style>
