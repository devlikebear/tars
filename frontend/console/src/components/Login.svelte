<script lang="ts">
  import { APIRequestError, loginAuth, loginWithPairingCode } from '../lib/api'
  import type { AuthWhoamiResponse } from '../lib/types'

  interface Props {
    onLogin: (auth: AuthWhoamiResponse) => void
  }

  let { onLogin }: Props = $props()

  function isTailscaleHost(): boolean {
    if (typeof window === 'undefined') return false
    return window.location.hostname.toLowerCase().endsWith('.ts.net')
  }

  let mode = $state<'password' | 'pairing'>('password')
  let username = $state<'admin' | 'user'>(isTailscaleHost() ? 'user' : 'admin')
  let password = $state('')
  let pairingCode = $state('')
  let loading = $state(false)
  let error = $state('')

  let remoteHost = $derived(isTailscaleHost())

  function messageForError(err: unknown): string {
    if (err instanceof APIRequestError) {
      const code = String(err.payload?.code ?? '')
      if (code === 'remote_admin_forbidden') return 'Remote admin login is disabled. Sign in as user on mobile.'
      if (code === 'login_not_allowed') return err.message
      if (code === 'invalid_credentials') return 'Username or password did not match.'
      if (code === 'invalid_pairing_code') return 'Pairing code is invalid or expired.'
      if (code === 'login_locked' || code === 'pairing_locked') return 'Too many attempts. Try again in a minute.'
      return err.message
    }
    return err instanceof Error ? err.message : 'Login failed.'
  }

  async function submitPassword() {
    if (loading) return
    error = ''
    loading = true
    try {
      const auth = await loginAuth({ username, password })
      onLogin(auth)
    } catch (err) {
      error = messageForError(err)
    } finally {
      loading = false
    }
  }

  async function submitPairing() {
    if (loading) return
    error = ''
    loading = true
    try {
      const auth = await loginWithPairingCode({ code: pairingCode })
      onLogin(auth)
    } catch (err) {
      error = messageForError(err)
    } finally {
      loading = false
    }
  }

  function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    if (mode === 'password') {
      void submitPassword()
    } else {
      void submitPairing()
    }
  }
</script>

<main class="login-page">
  <section class="login-panel" aria-label="TARS login">
    <div class="login-brand">
      <img src="/console/tars-icon.png" alt="" width="38" height="38" />
      <div>
        <h1>TARS</h1>
        <p>Sign in to continue</p>
      </div>
    </div>

    <div class="login-tabs" role="tablist" aria-label="Login method">
      <button type="button" class:active={mode === 'password'} onclick={() => { mode = 'password'; error = '' }}>Password</button>
      <button type="button" class:active={mode === 'pairing'} onclick={() => { mode = 'pairing'; error = '' }}>Pairing code</button>
    </div>

    <form class="login-form" onsubmit={handleSubmit}>
      {#if mode === 'password'}
        <label>
          <span>Account</span>
          <select bind:value={username}>
            <option value="user">User</option>
            <option value="admin" disabled={remoteHost}>Admin</option>
          </select>
        </label>
        {#if remoteHost}
          <p class="login-note">Mobile and Tailscale access only allow the user account.</p>
        {/if}
        <label>
          <span>Password</span>
          <input bind:value={password} type="password" autocomplete="current-password" />
        </label>
      {:else}
        <label>
          <span>Pairing code</span>
          <input bind:value={pairingCode} inputmode="numeric" autocomplete="one-time-code" placeholder="000000" />
        </label>
        <p class="login-note">Pairing codes create a user session and expire after a short window.</p>
      {/if}

      {#if error}
        <div class="login-error" role="alert">{error}</div>
      {/if}

      <button type="submit" class="login-submit" disabled={loading}>
        {loading ? 'Signing in...' : 'Sign in'}
      </button>
    </form>
  </section>
</main>

<style>
  .login-page {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: var(--space-6);
    background:
      linear-gradient(140deg, rgba(224, 145, 69, 0.16), rgba(44, 62, 80, 0.08)),
      var(--surface-base);
  }

  .login-panel {
    width: min(420px, 100%);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    padding: var(--space-6);
    box-shadow: 0 22px 70px rgba(0, 0, 0, 0.35);
  }

  .login-brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-5);
  }

  .login-brand img {
    border-radius: var(--radius-md);
    background: rgba(224, 145, 69, 0.12);
    border: 1px solid rgba(224, 145, 69, 0.28);
    padding: 4px;
  }

  .login-brand h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--text-xl);
    letter-spacing: 0;
  }

  .login-brand p {
    margin: 2px 0 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .login-tabs {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    padding: 3px;
    background: var(--surface-elevated);
    border-radius: var(--radius-md);
    margin-bottom: var(--space-5);
  }

  .login-tabs button {
    border: 0;
    border-radius: calc(var(--radius-md) - 2px);
    background: transparent;
    color: var(--text-secondary);
    padding: 8px var(--space-3);
    font: inherit;
    cursor: pointer;
  }

  .login-tabs button.active {
    background: var(--surface);
    color: var(--text-primary);
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.22);
  }

  .login-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  label {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  input,
  select {
    width: 100%;
    min-height: 40px;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle);
    background: var(--surface-elevated);
    color: var(--text-primary);
    padding: 0 var(--space-3);
    font: inherit;
  }

  .login-note {
    margin: calc(-1 * var(--space-2)) 0 0;
    color: var(--text-ghost);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  .login-error {
    border: 1px solid rgba(239, 68, 68, 0.34);
    background: rgba(239, 68, 68, 0.1);
    color: var(--error);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .login-submit {
    min-height: 42px;
    border: 0;
    border-radius: var(--radius-md);
    background: var(--primary);
    color: #141414;
    font-weight: 700;
    cursor: pointer;
  }

  .login-submit:disabled {
    opacity: 0.65;
    cursor: wait;
  }
</style>
