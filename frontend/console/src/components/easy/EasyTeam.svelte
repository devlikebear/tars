<script lang="ts">
  // The workspace's left column: the five bots with what each is doing,
  // and the team's log — fixed sentences built by easyView from what the
  // server keeps, with the leader's questions in it. A question's choices
  // answer its decision card (onAnswer); the "you decide" choice sends the
  // server's own words (EASY_YOU_DECIDE).
  import { t } from '../../i18n'
  import { EASY_BOTS, EASY_YOU_DECIDE, type EasyView } from '../../lib/easy'

  let {
    view,
    answering = '',
    onAnswer,
  }: { view: EasyView; answering?: string; onAnswer: (cardId: string, answer: string) => void } = $props()

  // The first letter of a bot's name, as its face.
  const face = (bot: (typeof EASY_BOTS)[number]) => Array.from($t.easy.bots[bot].name)[0] ?? ''
</script>

<div class="team">
  <ul class="bots" aria-label={$t.easy.project.team}>
    {#each EASY_BOTS as bot (bot)}
      <li class="bot state-{view.bots[bot]}" data-testid="easy-bot-{bot}" data-state={view.bots[bot]} title={$t.easy.bots[bot].role}>
        <span class="face" aria-hidden="true">{view.bots[bot] === 'done' ? '✓' : face(bot)}</span>
        <span class="name">{$t.easy.bots[bot].name}</span>
        <span class="state">{$t.easy.botState[view.bots[bot]]}</span>
      </li>
    {/each}
  </ul>

  <ol class="log" aria-label={$t.easy.workspace.conversation} data-testid="easy-log">
    {#each view.log as entry, i (i)}
      {#if entry.who === 'you'}
        <li class="you" data-testid="easy-log-you" aria-label={$t.easy.workspace.log[entry.line === 'change' ? 'change' : 'idea']}>{entry.text}</li>
      {:else}
        <li class="said" data-testid="easy-log-line" data-line={entry.line}>
          <b class="who">{$t.easy.bots[entry.who].name}</b>
          <span>
            {#if entry.line === 'checked'}
              {$t.easy.workspace.log.checked(entry.passed ?? 0, entry.total ?? 0)}
            {:else if entry.line !== 'idea' && entry.line !== 'change'}
              {$t.easy.workspace.log[entry.line]}
            {/if}
          </span>
          {#if entry.line === 'asked'}
            {#each view.questions as q (q.cardId)}
              {@const choices = q.youDecide ? [...q.options, EASY_YOU_DECIDE] : q.options}
              <fieldset class="question" data-testid="easy-question" data-answered={q.answer ? 'true' : 'false'} disabled={Boolean(q.answer) || answering !== ''}>
                <legend>{q.question}</legend>
                <div class="choices">
                  {#each choices as choice (choice)}
                    <button
                      type="button"
                      class="choice"
                      class:chosen={q.answer === choice}
                      aria-pressed={q.answer === choice}
                      data-testid="easy-question-choice"
                      onclick={() => onAnswer(q.cardId, choice)}
                    >
                      {choice === EASY_YOU_DECIDE && q.youDecide && !q.options.includes(choice) ? $t.easy.workspace.questions.youDecide : choice}
                    </button>
                  {/each}
                </div>
                {#if answering === q.cardId}<span class="sending">{$t.easy.workspace.questions.sending}</span>{/if}
              </fieldset>
            {/each}
          {/if}
        </li>
      {/if}
    {/each}
  </ol>
</div>

<style>
  .team {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .bots {
    list-style: none;
    margin: 0;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: var(--space-1);
    text-align: center;
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  .bot {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }
  .face {
    width: 32px;
    height: 32px;
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    display: grid;
    place-items: center;
    font-size: var(--text-sm);
    box-sizing: border-box;
  }
  .state {
    color: var(--text-tertiary);
    font-size: 11px;
  }
  .state-working,
  .state-done {
    color: var(--primary-text);
  }
  .state-working .face {
    background: rgba(var(--primary-rgb), 0.12);
    border: 1.5px solid var(--primary);
    animation: pulse 1.6s ease-in-out infinite;
  }
  .state-done .face {
    background: rgba(var(--primary-rgb), 0.12);
  }
  .state-needs_you {
    color: var(--warning);
  }
  .state-needs_you .face {
    background: var(--warning-muted);
    border: 1.5px solid var(--warning);
  }
  .state-later {
    opacity: 0.55;
  }
  .log {
    list-style: none;
    margin: 0;
    padding: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    font-size: var(--text-base);
    line-height: 1.55;
    overflow-y: auto;
  }
  .you {
    align-self: flex-end;
    max-width: 84%;
    background: var(--surface-active);
    border-radius: var(--radius-md) var(--radius-md) 2px var(--radius-md);
    padding: var(--space-2) var(--space-3);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .said {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .who {
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--primary-text);
  }
  .question {
    margin: 0;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    background: var(--surface-base);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }
  .question legend {
    float: left;
    padding: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .choices {
    clear: both;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .choice {
    font: inherit;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    border: 1px solid var(--border-default);
    background: transparent;
    border-radius: var(--radius-sm);
    padding: 6px var(--space-3);
    min-height: 32px;
    cursor: pointer;
  }
  .choice:hover:not(:disabled) {
    color: var(--text-primary);
    border-color: var(--border-strong);
  }
  .choice.chosen {
    color: var(--primary-contrast);
    background: var(--primary);
    border-color: var(--primary);
    font-weight: 600;
  }
  .question:disabled .choice:not(.chosen) {
    opacity: 0.6;
    cursor: default;
  }
  .sending {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  @keyframes pulse {
    50% {
      opacity: 0.6;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .state-working .face {
      animation: none;
    }
  }
</style>
