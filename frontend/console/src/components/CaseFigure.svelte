<script lang="ts">
  // CASE's body (case-companion phase 1.5): a codex-drawn robot from one
  // sprite sheet, with the face drawn in CSS on its screen and the state
  // colour on a CSS antenna LED. Everything here is decided by classes on
  // `.companion-body` — `expr-*` picks the face and the pose frame, `act-*`
  // and `play-*` the motion — so a clone with a different `expr-*` class
  // draws that expression (e2e/companion-expressions.spec.ts relies on it).
  // CompanionPet.svelte owns the state, timers and listeners.
  import sheetUrl from '../assets/case/case-bot.webp'
  import { caseFigureStyle } from '../lib/caseFigure'
  import { caseSprite } from '../lib/caseSprite.generated'
  import type { CompanionAction, CompanionExpression, CompanionIdleKind } from '../lib/companion'

  interface Props {
    expression: CompanionExpression
    action?: CompanionAction | null
    idlePlay?: CompanionIdleKind | null
    gaze?: { x: number; y: number }
    onanimationend?: (event: AnimationEvent) => void
  }

  let { expression, action = null, idlePlay = null, gaze = { x: 0, y: 0 }, onanimationend }: Props = $props()

  const figureStyle = caseFigureStyle(sheetUrl, caseSprite)
</script>

<span
  class={`companion-body expr-${expression}${action ? ` act-${action}` : ''}${idlePlay ? ` play-${idlePlay}` : ''}`}
  style={figureStyle}
  aria-hidden="true"
  {onanimationend}
>
  <span class="companion-sprite"></span>
  <span class="companion-screen">
    <span class="companion-brows">
      <span class="companion-brow companion-brow-left"></span>
      <span class="companion-brow companion-brow-right"></span>
    </span>
    <span class="companion-eyes" style={`--eye-x: ${gaze.x}px; --eye-y: ${gaze.y}px;`}>
      <span class="companion-eye companion-eye-left"></span>
      <span class="companion-eye companion-eye-right"></span>
    </span>
    <span class="companion-progress"></span>
  </span>
  <span class="companion-gem"></span>
  <span class="companion-zzz" aria-hidden="true">z</span>
</span>

<style>
  /* The colours are local to CASE, never global tokens: nothing outside
     CASE uses them. The eyes are always the one signal green — a state
     colour rides only on the antenna LED (.companion-gem), so two
     expressions never differ by colour alone (DESIGN.md). */
  .companion-body {
    --case-eye: #3ee07f;
    --case-eye-glow: rgba(62, 224, 127, 0.65);
    --case-gem-neutral: #8a877d;
    --case-gem-working: #f2a93b;
    --case-gem-alert: #ff7a59;
    --case-gem-happy: #9bd77a;
    --case-gem-sleepy: #4a4842;
    /* The pose frame and the LED position that goes with it. The
       `--case-*` values come from the inline style (lib/caseFigure.ts,
       generated from the sprite sheet); the expression rules below only
       pick which frame. */
    --frame: var(--case-frame-base);
    --led-x: var(--case-led-base-x);
    --led-y: var(--case-led-base-y);
    --led-r: var(--case-led-base-r);
    position: relative;
    width: 64px;
    height: 64px;
    transform-origin: 50% 100%;
  }

  .companion-body span {
    position: absolute;
    display: block;
  }

  .companion-body.expr-alert {
    --frame: var(--case-frame-alert);
    --led-x: var(--case-led-alert-x);
    --led-y: var(--case-led-alert-y);
    --led-r: var(--case-led-alert-r);
  }

  .companion-body.expr-working {
    --frame: var(--case-frame-working);
    --led-x: var(--case-led-working-x);
    --led-y: var(--case-led-working-y);
    --led-r: var(--case-led-working-r);
  }

  .companion-body.expr-greeting {
    --frame: var(--case-frame-wave);
    --led-x: var(--case-led-wave-x);
    --led-y: var(--case-led-wave-y);
    --led-r: var(--case-led-wave-r);
  }

  .companion-sprite {
    inset: 0;
    background-image: var(--case-sheet);
    background-repeat: no-repeat;
    background-size: var(--case-sheet-width) 100%;
    background-position: var(--frame) 0;
  }

  /* The face lives on the sprite's screen. Everything inside is sized in
     percent of the screen, so it follows the sheet if it is regenerated. */
  .companion-screen {
    left: var(--case-screen-x);
    top: var(--case-screen-y);
    width: var(--case-screen-w);
    height: var(--case-screen-h);
  }

  .companion-brows,
  .companion-eyes {
    inset: 0;
  }

  .companion-brow {
    top: 10%;
    width: 24%;
    height: max(2px, 9%);
    border-radius: 1px;
    background: var(--case-eye);
    /* Hidden by default: a brow only means something on alert/upset/wary
       below. Everywhere else it is a line above the eyes for no reason. */
    opacity: 0;
  }

  .companion-brow-left {
    left: 19%;
  }

  .companion-brow-right {
    left: 57%;
  }

  .companion-eye {
    top: 29%;
    width: 20%;
    height: 42%;
    border-radius: 30%;
    background: var(--case-eye);
    box-shadow: 0 0 3px var(--case-eye-glow);
    animation: companionBlink 5.6s infinite;
  }

  .companion-eye-left {
    left: 21%;
  }

  .companion-eye-right {
    left: 59%;
  }

  /* Cursor gaze (#1191): the eyes lean toward a nearby pointer.
     `translate` is a separate property from `transform`, so it composes
     with an expression's own nudge, and the blink animates `scale`, a
     third one. The component sets `--eye-x`/`--eye-y` on .companion-eyes
     (lib/companion.ts `companionGazeOffset`, already clamped to 3px),
     scaled down here to stay on the screen. Scoped to the four expressions
     whose eyes are open shapes: this selector, not just the component's
     `companionGazeApplies` check, keeps a stale offset off happy/upset/
     sleepy/greeting. */
  .companion-body:not(.expr-happy):not(.expr-upset):not(.expr-sleepy):not(.expr-greeting) .companion-eye {
    translate: calc(var(--eye-x, 0px) * 0.6) calc(var(--eye-y, 0px) * 0.6);
  }

  /* working's progress line: hidden everywhere else. */
  .companion-body .companion-progress {
    display: none;
    left: 28%;
    top: 76%;
    width: 44%;
    height: max(2px, 8%);
    border-radius: 1px;
    background: linear-gradient(90deg, transparent 0 30%, var(--case-eye) 30% 60%, transparent 60%) 0 0 / 200% 100%;
    box-shadow: inset 0 0 0 1px rgba(62, 224, 127, 0.25);
  }

  /* The antenna LED: the one place a state colour rides (DESIGN.md). It
     sits over the sprite's own grey LED, a little larger so none of it
     shows through. */
  .companion-gem {
    left: calc(var(--led-x) - var(--led-r) * 1.25);
    top: calc(var(--led-y) - var(--led-r) * 1.25);
    width: calc(var(--led-r) * 2.5);
    height: calc(var(--led-r) * 2.5);
    border-radius: 50%;
    background: var(--case-gem-neutral);
  }

  /* The sleepy 'z' — hidden everywhere else. Decoration, never a hit
     target (.companion-figure is pointer-events: none). */
  .companion-body .companion-zzz {
    display: none;
    right: 6px;
    top: 2px;
    font-family: var(--font-mono);
    font-size: 8px;
    font-weight: 600;
    line-height: 1;
    color: var(--text-secondary);
    pointer-events: none;
  }

  /* neutral (idle, the default): rounded-rectangle eyes, blinking, no
     brows, a grey LED — the base shapes above as-is. */
  .expr-neutral .companion-gem {
    background: var(--case-gem-neutral);
  }

  /* working (a turn is running): the eyes narrow and glance down at a
     running progress line, the arms come forward (pose frame), and the
     LED turns amber and pulses for as long as the turn runs. */
  .expr-working .companion-eye {
    top: 44%;
    width: 22%;
    height: 16%;
    border-radius: 40%;
  }

  .expr-working .companion-eye-left {
    left: 20%;
  }

  .expr-working .companion-eye-right {
    left: 58%;
  }

  .expr-working .companion-progress {
    display: block;
    animation: companionProgress 1.2s linear infinite;
  }

  .expr-working .companion-gem {
    background: var(--case-gem-working);
    animation: companionGemPulse 1.8s ease-in-out infinite;
  }

  /* alert (something is waiting on you): big eyes that stop blinking,
     both brows lift, the antenna shoots up and the arms swing out (pose
     frame), an orange LED. */
  .expr-alert .companion-eye {
    top: 22%;
    width: 24%;
    height: 56%;
    animation: none;
  }

  .expr-alert .companion-eye-left {
    left: 19%;
  }

  .expr-alert .companion-eye-right {
    left: 57%;
  }

  .expr-alert .companion-brow {
    top: 5%;
    opacity: 1;
  }

  .expr-alert .companion-brow-left {
    transform: rotate(-12deg);
  }

  .expr-alert .companion-brow-right {
    transform: rotate(12deg);
  }

  .expr-alert .companion-gem {
    background: var(--case-gem-alert);
    box-shadow: 0 0 3px var(--case-gem-alert);
  }

  /* happy (a turn just finished): ∩ arc eyes, a green LED. */
  .expr-happy .companion-eye,
  .expr-greeting .companion-eye-left {
    top: 32%;
    width: 22%;
    height: 30%;
    background: transparent;
    box-shadow: none;
    border: 2.5px solid var(--case-eye);
    border-bottom: 0;
    border-radius: 50% 50% 0 0 / 100% 100% 0 0;
    filter: drop-shadow(0 0 2px var(--case-eye-glow));
    animation: none;
  }

  .expr-happy .companion-gem {
    background: var(--case-gem-happy);
  }

  /* upset (a failure): small dot eyes, brows furrowed down toward the
     middle — alert's LED colour, so it differs from alert in eye size,
     brow direction and pose at once (DESIGN.md). */
  .expr-upset .companion-eye {
    top: 50%;
    width: 11%;
    height: 20%;
    border-radius: 50%;
    animation: none;
  }

  .expr-upset .companion-eye-left {
    left: 27%;
  }

  .expr-upset .companion-eye-right {
    left: 62%;
  }

  .expr-upset .companion-brow {
    top: 26%;
    opacity: 1;
  }

  .expr-upset .companion-brow-left {
    transform: rotate(20deg);
  }

  .expr-upset .companion-brow-right {
    transform: rotate(-20deg);
  }

  .expr-upset .companion-gem {
    background: var(--case-gem-alert);
  }

  /* wary (a warning): one eye squinted, both glancing sideways, one brow
     raised and one lowered — asymmetric on purpose, a symmetric change
     reads as "surprised", not "suspicious". */
  .expr-wary .companion-eye {
    animation: none;
  }

  .expr-wary .companion-eye-left {
    left: 29%;
    top: 44%;
    height: 14%;
  }

  .expr-wary .companion-eye-right {
    left: 67%;
    top: 30%;
    height: 36%;
  }

  .expr-wary .companion-brow {
    opacity: 1;
  }

  .expr-wary .companion-brow-left {
    left: 27%;
    top: 28%;
    transform: rotate(8deg);
  }

  .expr-wary .companion-brow-right {
    left: 65%;
    top: 8%;
    transform: rotate(-10deg);
  }

  .expr-wary .companion-gem {
    background: var(--case-gem-working);
  }

  /* sleepy (quiet for a while): eyes shut to dim lines, the whole robot
     leans (a static `rotate`, so it holds under reduced motion too),
     breathes slowly, and a 'z' drifts up; a dark LED. */
  .expr-sleepy .companion-eye {
    top: 55%;
    height: 6%;
    opacity: 0.45;
    box-shadow: none;
    animation: none;
  }

  .expr-sleepy.companion-body {
    rotate: -5deg;
    animation: companionBreathe 3.6s ease-in-out infinite;
  }

  .expr-sleepy .companion-gem {
    background: var(--case-gem-sleepy);
  }

  .expr-sleepy .companion-zzz {
    display: block;
    animation: companionZzzFloat 2.4s ease-in-out infinite;
  }

  /* greeting (returning, or saying hello): a wink (one arc eye, shared
     with happy above, one wide-open eye) and a raised arm (pose frame). */
  .expr-greeting .companion-eye-right {
    top: 26%;
    width: 22%;
    height: 48%;
    animation: none;
  }

  .expr-greeting .companion-gem {
    background: var(--case-gem-happy);
  }

  .companion-body.act-bounce {
    /* Actions (#1191): a one-shot class the component (lib/companion.ts
       `companionActionFor`) adds exactly once per transition into its
       expression, and clears on its own `animationend`. bounce, nod,
       shake and tilt move the whole robot; wave flips the raised arm
       between its two sprite frames. */
    animation: companionActBounce 0.5s var(--ease-out);
  }

  .companion-body.act-nod {
    animation: companionActNod 0.6s var(--ease-out);
  }

  .companion-body.act-shake {
    animation: companionActShake 0.5s var(--ease-out);
  }

  .companion-body.act-tilt {
    animation: companionActTilt 0.6s var(--ease-out);
  }

  .companion-body.act-wave .companion-sprite {
    animation: companionActWave 0.9s step-end 2;
  }

  .companion-body.expr-neutral.play-look .companion-eye {
    /* Idle play (#1191): look/yawn/wink, a few seconds apart while CASE
       is neutral and idle (lib/companion.ts `companionNextIdle`).
       Decoration only — it never touches the expression or the badge.
       Requiring `.expr-neutral` here, not just the play-* class, is what
       stops a stale look/yawn/wink from fighting a real expression's own
       eye shape (the component also clears it in JS). */
    animation: companionLookAround 1.4s ease-in-out;
  }

  .companion-body.expr-neutral.play-yawn .companion-eye {
    animation: companionYawn 1.2s ease-in-out;
  }

  .companion-body.expr-neutral.play-wink .companion-eye-right {
    animation: companionWink 0.5s ease-in-out;
  }

  @keyframes companionBlink {
    0%, 44%, 50%, 100% { scale: 1 1; }
    47% { scale: 1 0.12; }
  }

  /* working's continuous LED pulse and progress line. */
  @keyframes companionGemPulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.45; }
  }

  @keyframes companionProgress {
    from { background-position: 100% 0; }
    to { background-position: -100% 0; }
  }

  /* sleepy's continuous breathing + drifting 'z'. */
  @keyframes companionBreathe {
    0%, 100% { transform: scale(1); }
    50% { transform: scale(1.03); }
  }

  @keyframes companionZzzFloat {
    0% { opacity: 0; transform: translateY(0); }
    30% { opacity: 0.75; }
    100% { opacity: 0; transform: translateY(-8px); }
  }

  /* One-shot actions (#1191), one per expression that has one: alert
     bounces, happy nods, upset shakes, wary tilts, greeting waves. */
  @keyframes companionActBounce {
    0%, 100% { transform: translateY(0); }
    30% { transform: translateY(-8px); }
    55% { transform: translateY(0); }
    75% { transform: translateY(-3px); }
  }

  @keyframes companionActNod {
    0%, 100% { transform: translateY(0) scaleY(1); }
    35% { transform: translateY(2px) scaleY(0.96); }
    70% { transform: translateY(-1px) scaleY(1.01); }
  }

  @keyframes companionActShake {
    0%, 100% { transform: translateX(0); }
    20% { transform: translateX(-4px); }
    40% { transform: translateX(4px); }
    60% { transform: translateX(-2px); }
    80% { transform: translateX(2px); }
  }

  @keyframes companionActTilt {
    0%, 100% { transform: rotate(0deg); }
    40% { transform: rotate(-9deg); }
    75% { transform: rotate(4deg); }
  }

  /* Flips between the two raised-arm frames; step-end holds each frame
     until the next keyframe instead of sliding the sheet between them. */
  @keyframes companionActWave {
    0%, 50%, 100% { background-position: var(--case-frame-wave) 0; }
    25%, 75% { background-position: var(--case-frame-wave2) 0; }
  }

  /* Idle play (#1191): look/yawn/wink, purely decorative. */
  @keyframes companionLookAround {
    0%, 100% { transform: translateX(0); }
    25% { transform: translateX(-1.5px); }
    50% { transform: translateX(0); }
    75% { transform: translateX(1.5px); }
  }

  @keyframes companionYawn {
    0%, 100% { scale: 1 1; }
    40%, 60% { scale: 1.15 0.2; }
  }

  @keyframes companionWink {
    0%, 100% { scale: 1 1; }
    50% { scale: 1 0.1; }
  }

  /* Reduced motion stops every animation here — every selector above with
     a repeating or one-shot animation gets its own `animation: none` entry
     here, named exactly (tests/companionPet.test.ts checks this
     one-for-one). The expression itself still reads from shape alone. */
  @media (prefers-reduced-motion: reduce) {
    .companion-eye,
    .expr-working .companion-progress,
    .expr-working .companion-gem,
    .expr-sleepy.companion-body,
    .expr-sleepy .companion-zzz,
    .companion-body.act-bounce,
    .companion-body.act-nod,
    .companion-body.act-shake,
    .companion-body.act-tilt,
    .companion-body.act-wave .companion-sprite,
    .companion-body.expr-neutral.play-look .companion-eye,
    .companion-body.expr-neutral.play-yawn .companion-eye,
    .companion-body.expr-neutral.play-wink .companion-eye-right {
      animation: none;
    }
  }
</style>
