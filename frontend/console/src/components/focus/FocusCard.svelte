<script lang="ts">
  // One card of the deck (ADR §7) and its actions: a gate (G1 plan; the
  // blocked gate retry / instruct / stop; later gates approve / request
  // changes / stop), a decision (option buttons or a
  // free answer), a finding (fix / dismiss / ask, with its diff excerpt),
  // and the informational cards —
  // verification failure, report, change, notice — which are acknowledged.
  import { t } from '../../i18n'
  import type {
    FocusBlocked,
    FocusCard,
    FocusChangeTurnPayload,
    FocusDecision,
    FocusFinding,
    FocusGateAction,
    FocusPlan,
    FocusPRDraft,
    FocusReport,
  } from '../../lib/types'
  import { excerptLines } from '../../lib/focus'
  import FocusChangeCard from './FocusChangeCard.svelte'
  import FocusPlanGate from './FocusPlanGate.svelte'

  interface Props {
    card: FocusCard
    // The pipeline's open gate ('' when none).
    openGate: string
    busy: boolean
    onGate: (gate: string, action: FocusGateAction, note?: string, edits?: FocusPlan) => void
    onDecide: (card: FocusCard, decision: string) => void
    // Opens the card's Q&A drawer; absent hides a finding's Ask.
    onAsk?: () => void
  }

  let { card, openGate, busy, onGate, onDecide, onAsk }: Props = $props()

  let answer = $state('')
  let asking = $state(false)
  let note = $state('')

  let decided = $derived(card.state === 'decided')
  let isBlocked = $derived(card.kind === 'gate' && isBlockedPayload(card.payload))
  let title = $derived(
    card.kind === 'gate' && card.stage === 'plan'
      ? $t.focus.gate.planTitle
      : isBlocked
        ? blockedTitle(card.payload as FocusBlocked)
        : card.kind === 'change'
          ? $t.focus.change.turnTitle(card.turn)
          : card.title,
  )
  // The open gate's card is the newest undecided gate card.
  let gateOpen = $derived(card.kind === 'gate' && !decided && !!openGate)
  let isPRDraft = $derived(card.kind === 'report' && isDraft(card.payload))

  function isDraft(payload: unknown): payload is FocusPRDraft {
    return !!payload && typeof payload === 'object' && 'body' in payload && !('summary' in payload)
  }

  function isBlockedPayload(payload: unknown): payload is FocusBlocked {
    return !!payload && typeof payload === 'object' && typeof (payload as FocusBlocked).reason === 'string'
  }

  function blockedTitle(b: FocusBlocked): string {
    if (b.reason === 'interrupted') return $t.focus.gate.interruptedTitle
    if (b.reason === 'turn_failed') return $t.focus.gate.turnFailedTitle
    return card.stage === 'review' ? $t.focus.gate.reviewBlockedTitle : $t.focus.gate.blockedTitle
  }

  function blockedReason(b: FocusBlocked): string {
    switch (b.reason) {
      case 'interrupted':
        return b.verify ? $t.focus.gate.blockedReason.interruptedVerify : $t.focus.gate.blockedReason.interrupted
      case 'turn_failed':
        return $t.focus.gate.blockedReason.turn_failed
      case 'limit':
        return card.stage === 'review' ? $t.focus.gate.blockedReason.reviewLimit(b.iteration, b.limit) : $t.focus.gate.blockedReason.limit(b.iteration, b.limit)
      case 'repeated':
        return $t.focus.gate.blockedReason.repeated
      case 'no_progress':
        return $t.focus.gate.blockedReason.no_progress
      default:
        return b.reason
    }
  }

  function asPlan(payload: unknown): FocusPlan | null {
    return payload && typeof payload === 'object' && Array.isArray((payload as FocusPlan).tasks) ? (payload as FocusPlan) : null
  }

  function errorsOf(payload: unknown): string[] {
    const errors = (payload as { errors?: unknown } | null)?.errors
    return Array.isArray(errors) ? errors.filter((e): e is string => typeof e === 'string') : []
  }

  // Facts of a verification failure (P2): command, exit code, excerpt, and
  // the build round it ended.
  function failureFacts(payload: unknown): { command: string; exit: string; excerpt: string; timedOut: boolean; round: number } {
    const p = (payload ?? {}) as Record<string, unknown>
    return {
      command: typeof p.command === 'string' ? p.command : '',
      exit: p.exit_code !== undefined ? String(p.exit_code) : '',
      excerpt: typeof p.excerpt === 'string' ? p.excerpt : typeof p.output === 'string' ? p.output : '',
      timedOut: p.timed_out === true,
      round: typeof p.iteration === 'number' ? p.iteration : 0,
    }
  }

  function sendAnswer() {
    const text = answer.trim()
    if (!text) return
    onDecide(card, text)
    answer = ''
  }

  function decisionLabel(decision: string | undefined): string {
    switch (decision) {
      case 'approve':
        return $t.focus.gate.approve
      case 'request_changes':
        return $t.focus.gate.requestChanges
      case 'stop':
        return $t.focus.gate.stop
      case 'retry':
        return $t.focus.gate.retry
      case 'instruct':
        return $t.focus.gate.instruct
      case 'acknowledged':
        return $t.focus.card.acknowledged
      case 'fix':
        return $t.focus.finding.fix
      case 'dismiss':
        return $t.focus.finding.dismiss
      default:
        return decision ?? ''
    }
  }
</script>

<article class="focus-card kind-{card.kind}" class:decided data-testid="focus-card" data-kind={card.kind} data-card-id={card.id}>
  <header class="card-head">
    <span class="badge kind-badge">{$t.focus.kinds[card.kind] ?? card.kind}</span>
    <!-- Not the global .card-title (label caps): a card's title is a sentence. -->
    <h3 class="focus-card-title" title={title} data-content>{title}</h3>
    <span class="card-meta">
      {#if card.turn > 0}<span class="mono">{$t.focus.card.turn(card.turn)}</span>{/if}
      <span class="badge {card.state === 'unseen' ? 'badge-accent' : 'badge-default'}">{$t.focus.states[card.state]}</span>
    </span>
  </header>

  <div class="card-body">
    {#if card.kind === 'gate'}
      {#if card.stage === 'plan'}
        <FocusPlanGate
          cardId={card.id}
          plan={asPlan(card.payload)}
          open={gateOpen && openGate === 'plan'}
          {busy}
          onApprove={(edits) => onGate('plan', 'approve', undefined, edits)}
          onRequestChanges={(text) => onGate('plan', 'request_changes', text)}
          onStop={() => onGate('plan', 'stop')}
        />
      {:else if isBlocked}
        {@const b = card.payload as FocusBlocked}
        <p class="prose" data-testid="focus-blocked-reason">{blockedReason(b)}</p>
        {#if b.error}<pre class="mono" data-content>{b.error}</pre>{/if}
        {#if b.prompt}<p class="muted" data-content>{b.prompt}</p>{/if}
        {#if b.failure}
          {@const f = failureFacts(b.failure)}
          {#if f.command}<p class="mono" data-content>$ {f.command}{f.timedOut ? ` → ${$t.focus.failure.timedOut}` : f.exit ? ` → ${f.exit}` : ''}</p>{/if}
          {#if f.excerpt}<pre class="mono" data-content>{f.excerpt}</pre>{/if}
        {/if}
        {#if gateOpen && openGate === 'blocked'}
          {#if asking}
            <textarea rows="3" bind:value={note} placeholder={$t.focus.gate.instructPlaceholder} disabled={busy} data-testid="focus-blocked-note"></textarea>
            <div class="actions">
              <button type="button" class="btn btn-primary btn-sm" disabled={busy || !note.trim()} onclick={() => onGate('blocked', 'instruct', note.trim())} data-testid="focus-blocked-send">{$t.focus.gate.sendInstruct}</button>
              <button type="button" class="btn btn-ghost btn-sm" onclick={() => { asking = false }}>{$t.focus.gate.cancel}</button>
            </div>
          {:else}
            <div class="actions">
              <button type="button" class="btn btn-primary" disabled={busy} onclick={() => onGate('blocked', 'retry')} data-testid="focus-blocked-retry">{$t.focus.gate.retry}</button>
              <button type="button" class="btn btn-secondary" disabled={busy} onclick={() => { asking = true }} data-testid="focus-blocked-instruct">{$t.focus.gate.instruct}</button>
              <button type="button" class="btn btn-danger" disabled={busy} onclick={() => onGate('blocked', 'stop')}>{$t.focus.gate.stop}</button>
            </div>
          {/if}
        {/if}
      {:else if gateOpen}
        {#if asking}
          <textarea rows="3" bind:value={note} placeholder={$t.focus.gate.notePlaceholder} disabled={busy}></textarea>
          <div class="actions">
            <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !note.trim()} onclick={() => onGate(openGate, 'request_changes', note.trim())}>{$t.focus.gate.sendChanges}</button>
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => { asking = false }}>{$t.focus.gate.cancel}</button>
          </div>
        {:else}
          <div class="actions">
            <button type="button" class="btn btn-primary" disabled={busy} onclick={() => onGate(openGate, 'approve')}>{$t.focus.gate.approve}</button>
            <button type="button" class="btn btn-secondary" disabled={busy} onclick={() => { asking = true }}>{$t.focus.gate.requestChanges}</button>
            <button type="button" class="btn btn-danger" disabled={busy} onclick={() => onGate(openGate, 'stop')}>{$t.focus.gate.stop}</button>
          </div>
        {/if}
      {/if}
    {:else if card.kind === 'decision'}
      {@const d = card.payload as FocusDecision | undefined}
      <div class="options" role="group" aria-label={$t.focus.decision.answer}>
        {#each d?.options ?? [] as option, i (i)}
          <button
            type="button"
            class="btn option"
            class:btn-primary={decided && card.decision === option}
            class:btn-secondary={!(decided && card.decision === option)}
            disabled={busy || decided}
            data-testid="focus-decision-option"
            onclick={() => onDecide(card, option)}
          >
            {#if i < 9}<kbd class="mono">{i + 1}</kbd>{/if}
            <span data-content>{option}</span>
          </button>
        {/each}
      </div>
      {#if !decided}
        <div class="answer">
          <input type="text" bind:value={answer} placeholder={$t.focus.decision.answerPlaceholder} disabled={busy} aria-label={$t.focus.decision.answer} onkeydown={(e) => { if (e.key === 'Enter' && !e.isComposing) sendAnswer() }} />
          <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !answer.trim()} onclick={sendAnswer}>{$t.focus.decision.send}</button>
        </div>
      {/if}
    {:else if card.kind === 'finding'}
      {@const f = card.payload as FocusFinding | undefined}
      <p class="finding-loc">
        {#if f?.severity}
          <span class="badge {f.severity === 'high' ? 'badge-error' : f.severity === 'medium' ? 'badge-warning' : 'badge-default'}" data-testid="focus-finding-severity">{$t.focus.finding.severity[f.severity] ?? f.severity}</span>
        {/if}
        {#if f?.file}<span class="mono" data-content data-testid="focus-finding-loc">{f.file}{f.line ? `:${f.line}` : ''}</span>{/if}
      </p>
      {#if f?.scenario}
        <h4 class="label">{$t.focus.finding.scenario}</h4>
        <p class="prose" data-content>{f.scenario}</p>
      {/if}
      {#if f?.excerpt}
        <h4 class="label">{$t.focus.finding.diff}</h4>
        <pre class="mono excerpt" data-content data-testid="focus-finding-excerpt">{#each excerptLines(f.excerpt, f.line) as l, i (i)}<span class="ex-{l.kind}" class:ex-target={l.target}>{l.text}</span>{/each}</pre>
      {/if}
      {#if !decided}
        <div class="actions">
          <button type="button" class="btn btn-primary btn-sm" disabled={busy} onclick={() => onDecide(card, 'fix')} data-testid="focus-finding-fix">{$t.focus.finding.fix}</button>
          <button type="button" class="btn btn-secondary btn-sm" disabled={busy} onclick={() => onDecide(card, 'dismiss')} data-testid="focus-finding-dismiss">{$t.focus.finding.dismiss}</button>
          {#if onAsk}
            <button type="button" class="btn btn-ghost btn-sm" onclick={onAsk} data-testid="focus-finding-ask">{$t.focus.finding.ask} <kbd class="mono">?</kbd></button>
          {/if}
        </div>
      {/if}
    {:else if card.kind === 'failure'}
      {@const f = failureFacts(card.payload)}
      {#if f.command}<p class="mono" data-content>$ {f.command}{f.timedOut ? ` → ${$t.focus.failure.timedOut}` : f.exit ? ` → ${f.exit}` : ''}</p>{/if}
      {#if f.round}<p class="muted">{$t.focus.failure.round(f.round)}</p>{/if}
      {#if f.excerpt}<pre class="mono" data-content data-testid="focus-failure-excerpt">{f.excerpt}</pre>{/if}
    {:else if card.kind === 'report' && isPRDraft}
      {@const pr = card.payload as FocusPRDraft}
      <h4 class="label">{$t.focus.report.prTitle}</h4>
      <p class="prose" data-content>{pr.title}</p>
      <h4 class="label">{$t.focus.report.prBody}</h4>
      <pre class="prose-pre" data-content>{pr.body}</pre>
    {:else if card.kind === 'report'}
      {@const r = card.payload as FocusReport | undefined}
      <!-- The title is the summary's first line; show the rest only. -->
      {#if r?.summary && r.summary.trim() !== card.title}
        <p class="prose summary" data-content>{r.summary}</p>
      {/if}
      {#if r?.risks?.length}
        <h4 class="label">{$t.focus.report.risks}</h4>
        <ul class="risks">
          {#each r.risks as risk, i (i)}<li data-content>{risk}</li>{/each}
        </ul>
      {/if}
      {#if r?.decisions?.length}
        <p class="muted">{$t.focus.report.decisions(r.decisions.length)}</p>
      {/if}
    {:else if card.kind === 'change'}
      <FocusChangeCard change={card.payload as FocusChangeTurnPayload} />
    {:else if card.kind === 'notice'}
      {@const errors = errorsOf(card.payload)}
      {#if errors.length}
        <h4 class="label">{$t.focus.notice.errors}</h4>
        <ul class="risks">
          {#each errors as error, i (i)}<li class="mono" data-content>{error}</li>{/each}
        </ul>
      {/if}
    {/if}

    {#if (card.kind === 'report' || card.kind === 'change' || card.kind === 'failure' || card.kind === 'notice') && !decided}
      <div class="actions">
        <button type="button" class="btn btn-secondary btn-sm" disabled={busy} data-testid="focus-acknowledge" onclick={() => onDecide(card, 'acknowledged')}>{$t.focus.card.acknowledge}</button>
      </div>
    {/if}

    {#if decided && card.decision && card.kind !== 'decision'}
      <p class="decided-line">{$t.focus.card.decided(decisionLabel(card.decision))}</p>
    {/if}
  </div>
</article>

<style>
  .focus-card {
    min-width: 0;
    overflow-wrap: anywhere;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-5);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
  }

  .kind-gate:not(.decided),
  .kind-decision:not(.decided),
  .kind-finding:not(.decided) {
    border-color: rgba(var(--primary-rgb), 0.45);
  }

  .kind-failure:not(.decided),
  .kind-notice:not(.decided) {
    border-color: var(--warning);
  }

  .card-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .focus-card-title {
    flex: 1 1 0;
    min-width: 0;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-primary);
  }

  .card-meta {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .kind-badge {
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }

  .card-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  h4 {
    margin: 0;
  }

  .prose {
    margin: 0;
    color: var(--text-primary);
    line-height: var(--leading-normal);
    white-space: pre-wrap;
  }

  .prose-pre,
  pre {
    margin: 0;
    padding: var(--space-2);
    background: var(--surface-inset);
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
    white-space: pre-wrap;
    max-height: 320px;
    overflow: auto;
  }

  .risks {
    margin: 0;
    padding-left: var(--space-5);
    color: var(--text-secondary);
  }

  .muted,
  .decided-line {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .option {
    justify-content: flex-start;
    gap: var(--space-2);
    text-align: left;
  }

  kbd {
    padding: 0 var(--space-1);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .answer {
    display: flex;
    gap: var(--space-2);
  }

  .answer input,
  textarea {
    flex: 1;
    padding: var(--space-2);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .answer input:focus,
  textarea:focus {
    outline: none;
    border-color: var(--primary);
  }

  .finding-loc {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  /* A finding's diff excerpt: one line per span, the finding line marked. */
  .excerpt {
    white-space: pre;
    overflow-x: auto;
  }

  .excerpt span {
    display: block;
    min-width: max-content;
    padding: 0 var(--space-1);
  }

  .ex-hunk,
  .ex-meta {
    color: var(--text-tertiary);
  }

  .ex-add {
    color: var(--primary-text);
    background: rgba(var(--primary-rgb), 0.08);
  }

  .ex-del {
    color: var(--error);
    background: var(--error-muted);
  }

  .ex-target {
    box-shadow: inset 2px 0 0 var(--warning);
    font-weight: 600;
  }
</style>
