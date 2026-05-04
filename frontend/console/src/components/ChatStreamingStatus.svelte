<script lang="ts">
  interface Props {
    label: string
    elapsedLabel: string
    steps: readonly string[]
    currentStepIndex: number
    stepLabels: string[]
    locale: 'ko' | 'en'
    onToggleLocale: () => void
  }

  let {
    label,
    elapsedLabel,
    steps,
    currentStepIndex,
    stepLabels,
    locale,
    onToggleLocale,
  }: Props = $props()
</script>

<div class="chat-streaming-status">
  <span class="chat-status-line">
    {label}
    <span class="chat-status-elapsed">({elapsedLabel})</span>
    <span class="chat-status-dots" aria-hidden="true">
      <span></span>
      <span></span>
      <span></span>
    </span>
  </span>
  {#if currentStepIndex >= 0}
    <div class="chat-status-progress" aria-label="Status progress">
      {#each steps as _step, stepIdx (stepIdx)}
        <span
          class={`chat-status-progress-step ${stepIdx < currentStepIndex ? 'completed' : ''} ${stepIdx === currentStepIndex ? 'active' : ''}`}
          title={stepLabels[stepIdx]}
        >
          {stepLabels[stepIdx]}
        </span>
        {#if stepIdx < steps.length - 1}
          <span class="chat-status-progress-connector" aria-hidden="true"></span>
        {/if}
      {/each}
    </div>
  {/if}
  <button
    type="button"
    class="chat-status-locale-toggle"
    title={locale === 'ko' ? 'Switch to English status labels' : '상태 라벨을 한국어로 전환'}
    onclick={onToggleLocale}
  >
    {locale === 'ko' ? 'EN' : 'KR'}
  </button>
</div>

<style>
  .chat-streaming-status {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-ghost);
  }

  .chat-status-line {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chat-status-dots {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }

  .chat-status-dots span {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: currentColor;
    display: inline-block;
    opacity: 0.25;
    animation: chat-status-dot 0.8s infinite ease-in-out;
  }

  .chat-status-dots span:nth-child(2) {
    animation-delay: 0.1s;
  }

  .chat-status-dots span:nth-child(3) {
    animation-delay: 0.2s;
  }

  .chat-status-elapsed {
    font-size: 10px;
    color: var(--text-ghost);
    opacity: 0.9;
    margin-left: 2px;
  }

  .chat-status-progress {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .chat-status-progress-step {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--border-subtle);
    border-radius: 9999px;
    color: var(--text-ghost);
    font-size: 10px;
    padding: 1px 7px;
    line-height: 1.2;
    white-space: nowrap;
  }

  .chat-status-progress-step.completed {
    color: var(--text-secondary);
    border-color: var(--border-default);
    background: color-mix(in srgb, var(--surface-elevated) 80%, var(--text-secondary) 20%);
  }

  .chat-status-progress-step.active {
    color: var(--text-primary);
    border-color: var(--primary);
    font-weight: 600;
    background: color-mix(in srgb, var(--surface-elevated) 70%, var(--primary) 30%);
  }

  .chat-status-progress-connector {
    width: 6px;
    height: 1px;
    background: var(--border-subtle);
  }

  .chat-status-locale-toggle {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-elevated);
    color: var(--text-ghost);
    font-size: 10px;
    padding: 1px 6px;
    cursor: pointer;
    line-height: 1;
    min-height: 18px;
  }

  .chat-status-locale-toggle:hover {
    color: var(--text-secondary);
    border-color: var(--border-default);
  }

  @keyframes chat-status-dot {
    0%,
    80%,
    100% {
      transform: scale(1);
      opacity: 0.25;
    }
    40% {
      transform: scale(1.4);
      opacity: 1;
    }
  }
</style>
