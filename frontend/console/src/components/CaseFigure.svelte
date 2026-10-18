<script lang="ts">
  // CASE's body (case-companion phase 1.5): a codex-drawn robot from one
  // sprite sheet, with the face drawn in CSS on its screen and the state
  // colour on a CSS antenna LED. Everything here is decided by classes on
  // `.companion-body` — `expr-*` picks the face and the pose frame, `act-*`
  // and `play-*` the motion — so a clone with a different `expr-*` class
  // draws that expression (e2e/companion-expressions.spec.ts relies on it).
  // CompanionPet.svelte owns the state, timers and listeners.
  import sheetUrl from '../assets/case/case-bot.webp'
  import { caseFigureStyle, caseFrameFor } from '../lib/caseFigure'
  import { caseSprite } from '../lib/caseSprite.generated'
  import type { CompanionAction, CompanionExpression, CompanionIdleKind, CompanionTalk } from '../lib/companion'

  interface Props {
    expression: CompanionExpression
    action?: CompanionAction | null
    idlePlay?: CompanionIdleKind | null
    gaze?: { x: number; y: number }
    // CASE's own bubble answer streaming (lib/companion.ts companionTalk).
    talk?: CompanionTalk | null
    // Switched off by the user (companionSleepActive): the `off` pose, a
    // dark screen, no LED.
    asleep?: boolean
    // A one-shot power-on: arriving, or waking from sleep. The component
    // clears it on `companionBootFrame`'s animationend.
    booting?: boolean
    onanimationend?: (event: AnimationEvent) => void
  }

  let {
    expression,
    action = null,
    idlePlay = null,
    gaze = { x: 0, y: 0 },
    talk = null,
    asleep = false,
    booting = false,
    onanimationend,
  }: Props = $props()

  const figureStyle = caseFigureStyle(sheetUrl, caseSprite)

  // Pose cross-fade: the CSS switches the sprite frame instantly, and a
  // second layer showing the frame from a moment ago fades out on top. It
  // is removed on its own animationend; under reduced motion the CSS hides
  // it outright.
  let frame = $derived(caseFrameFor(expression, talk, asleep))
  let prevFrame = $state<string | null>(null)
  let fadeKey = $state(0)
  let lastFrame: string | null = null
  $effect(() => {
    const current = frame
    if (lastFrame !== null && lastFrame !== current) {
      prevFrame = lastFrame
      fadeKey += 1
    }
    lastFrame = current
  })

  function handleAnimationEnd(event: AnimationEvent) {
    if (event.animationName === 'companionPoseFade') prevFrame = null
    onanimationend?.(event)
  }
</script>

<span
  class={`companion-body expr-${expression}${action ? ` act-${action}` : ''}${idlePlay ? ` play-${idlePlay}` : ''}${talk ? ` talk-${talk}` : ''}${asleep ? ' is-asleep' : ''}${booting ? ' is-booting' : ''}`}
  style={figureStyle}
  aria-hidden="true"
  onanimationend={handleAnimationEnd}
>
  <span class="companion-sprite"></span>
  {#if prevFrame}
    {#key fadeKey}
      <span class="companion-sprite-prev" style={`background-position: var(--case-frame-${prevFrame}) 0`}></span>
    {/key}
  {/if}
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
    <span class="companion-wave">
      <span class="companion-wave-bar"></span>
      <span class="companion-wave-bar"></span>
      <span class="companion-wave-bar"></span>
    </span>
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
    /* One easing for every face change: a little overshoot, so a shape
       change reads as the robot reacting rather than a slide. */
    --case-ease: cubic-bezier(0.34, 1.56, 0.64, 1);
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

  /* The frame from a moment ago, fading out over the new one (the
     component adds it on a pose change and removes it on animationend). */
  .companion-sprite-prev {
    inset: 0;
    background-image: var(--case-sheet);
    background-repeat: no-repeat;
    background-size: var(--case-sheet-width) 100%;
    animation: companionPoseFade 0.18s ease-out forwards;
  }

  /* The face lives on the sprite's screen. Everything inside is sized in
     percent of the screen, so it follows the sheet if it is regenerated. */
  .companion-screen {
    left: var(--case-screen-x);
    top: var(--case-screen-y);
    width: var(--case-screen-w);
    height: var(--case-screen-h);
  }

  /* Power effects drawn on the screen glass, invisible at rest: the scan
     line that sweeps down when CASE boots (::before) and the line that
     collapses to a dot when it is switched off (::after). */
  .companion-screen::before,
  .companion-screen::after {
    content: '';
    position: absolute;
    left: 6%;
    right: 6%;
    height: 2px;
    border-radius: 1px;
    opacity: 0;
    pointer-events: none;
  }

  .companion-screen::before {
    top: 0;
    background: linear-gradient(90deg, transparent, var(--case-eye), transparent);
    box-shadow: 0 0 4px var(--case-eye-glow);
  }

  .companion-screen::after {
    top: calc(50% - 1px);
    background: var(--case-eye);
    box-shadow: 0 0 4px var(--case-eye-glow);
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
    transition: top 0.18s var(--case-ease), left 0.18s var(--case-ease), opacity 0.18s ease-out, transform 0.18s var(--case-ease);
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
    /* An expression change morphs the eyes instead of snapping them. */
    transition: top 0.18s var(--case-ease), left 0.18s var(--case-ease), width 0.18s var(--case-ease), height 0.18s var(--case-ease), border-radius 0.18s ease-out, opacity 0.18s ease-out;
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

  /* Talking's sound wave (three bars under the eyes): hidden everywhere
     else. */
  .companion-body .companion-wave {
    display: none;
    left: 33%;
    top: 68%;
    width: 34%;
    height: 22%;
  }

  .companion-wave-bar {
    bottom: 0;
    width: 22%;
    height: 100%;
    border-radius: 1px;
    background: var(--case-eye);
    transform-origin: 50% 100%;
    animation: companionWaveBar 0.42s ease-in-out infinite alternate;
  }

  .companion-wave-bar:nth-child(1) {
    left: 0;
    animation-delay: -0.3s;
  }

  .companion-wave-bar:nth-child(2) {
    left: 39%;
  }

  .companion-wave-bar:nth-child(3) {
    left: 78%;
    animation-delay: -0.15s;
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

  /* scan: the eyes sweep left and right and the LED blinks once — CASE
     checking on things. */
  .companion-body.expr-neutral.play-scan .companion-eye {
    animation: companionScan 1.6s ease-in-out;
  }

  .companion-body.expr-neutral.play-scan .companion-gem {
    animation: companionLedBlink 1.6s step-end;
  }

  /* Talking (lib/companion.ts companionTalk, F1): only ever alongside
     `working`, which CASE's own streaming answer forces. Both states stand
     up straight (base pose) and drop the progress line. Thinking glances
     up and sways the eyes; speaking shows a sound wave under the eyes. */
  .companion-body.expr-working.talk-thinking,
  .companion-body.expr-working.talk-speaking {
    --frame: var(--case-frame-base);
    --led-x: var(--case-led-base-x);
    --led-y: var(--case-led-base-y);
    --led-r: var(--case-led-base-r);
  }

  .companion-body.expr-working.talk-thinking .companion-progress,
  .companion-body.expr-working.talk-speaking .companion-progress {
    display: none;
  }

  .expr-working.talk-thinking .companion-eye {
    top: 16%;
    width: 18%;
    height: 32%;
    border-radius: 30%;
    animation: companionThink 1.6s ease-in-out infinite;
  }

  .expr-working.talk-thinking .companion-eye-left {
    left: 30%;
  }

  .expr-working.talk-thinking .companion-eye-right {
    left: 64%;
  }

  .expr-working.talk-speaking .companion-eye {
    top: 18%;
    width: 20%;
    height: 38%;
    border-radius: 30%;
  }

  .expr-working.talk-speaking .companion-eye-left {
    left: 21%;
  }

  .expr-working.talk-speaking .companion-eye-right {
    left: 59%;
  }

  .companion-body.expr-working.talk-speaking .companion-wave {
    display: block;
  }

  /* Asleep (F2, switched off by the user): the `off` pose — antenna folded,
     arms tucked — with a dark screen and an unlit LED, whatever the
     expression underneath. Still, too: no lean or breathing. */
  .companion-body.is-asleep {
    --frame: var(--case-frame-off);
    rotate: none;
    animation: none;
  }

  .companion-body.is-asleep .companion-brows,
  .companion-body.is-asleep .companion-eyes,
  .companion-body.is-asleep .companion-progress,
  .companion-body.is-asleep .companion-wave,
  .companion-body.is-asleep .companion-gem,
  .companion-body.is-asleep .companion-zzz {
    visibility: hidden;
  }

  .companion-body.is-asleep .companion-screen::after {
    animation: companionScreenOff 0.45s ease-in forwards;
  }

  /* Booting (arriving, or waking up): the `off` pose for a beat, then the
     real pose; a scan line sweeps the screen and the face and LED come on
     after it. greeting's wave waits for the boot to finish. */
  .companion-body.is-booting .companion-sprite {
    animation: companionBootFrame 0.7s step-end;
  }

  .companion-body.is-booting.act-wave .companion-sprite {
    animation: companionBootFrame 0.7s step-end, companionActWave 0.9s step-end 0.7s 2;
  }

  .companion-body.is-booting .companion-eyes,
  .companion-body.is-booting .companion-brows,
  .companion-body.is-booting .companion-gem {
    animation: companionBootOn 0.7s ease-out;
  }

  .companion-body.is-booting .companion-screen::before {
    animation: companionScanline 0.7s linear;
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

  @keyframes companionScan {
    0%, 100% { transform: translateX(0); }
    25% { transform: translateX(-2.5px); }
    75% { transform: translateX(2.5px); }
  }

  @keyframes companionLedBlink {
    0%, 45%, 55%, 100% { opacity: 1; }
    50% { opacity: 0.2; }
  }

  @keyframes companionPoseFade {
    from { opacity: 1; }
    to { opacity: 0; }
  }

  /* Talking (F1). */
  @keyframes companionThink {
    0%, 100% { transform: translateX(-1px); }
    50% { transform: translateX(1px); }
  }

  @keyframes companionWaveBar {
    from { transform: scaleY(0.25); }
    to { transform: scaleY(1); }
  }

  /* Sleep and boot (F2). */
  @keyframes companionScreenOff {
    0% { opacity: 1; transform: scale(1, 6); }
    55% { opacity: 1; transform: scale(1, 1); }
    85% { opacity: 1; transform: scale(0.06, 1); }
    100% { opacity: 0; transform: scale(0.02, 1); }
  }

  @keyframes companionBootFrame {
    0% { background-position: var(--case-frame-off) 0; }
    40%, 100% { background-position: var(--frame) 0; }
  }

  @keyframes companionBootOn {
    0%, 55% { opacity: 0; }
    100% { opacity: 1; }
  }

  @keyframes companionScanline {
    0%, 30% { opacity: 0; transform: translateY(0); }
    35% { opacity: 1; }
    85% { opacity: 1; transform: translateY(calc(var(--case-screen-h) - 2px)); }
    100% { opacity: 0; transform: translateY(calc(var(--case-screen-h) - 2px)); }
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
    .companion-body.expr-neutral.play-wink .companion-eye-right,
    .companion-body.expr-neutral.play-scan .companion-eye,
    .companion-body.expr-neutral.play-scan .companion-gem,
    .companion-sprite-prev,
    .companion-wave-bar,
    .expr-working.talk-thinking .companion-eye,
    .companion-body.is-asleep .companion-screen::after,
    .companion-body.is-booting .companion-sprite,
    .companion-body.is-booting.act-wave .companion-sprite,
    .companion-body.is-booting .companion-eyes,
    .companion-body.is-booting .companion-brows,
    .companion-body.is-booting .companion-gem,
    .companion-body.is-booting .companion-screen::before {
      animation: none;
    }

    /* The face changes shape at once instead of morphing. */
    .companion-eye,
    .companion-brow {
      transition: none;
    }

    /* No cross-fade: the new pose shows at once. */
    .companion-sprite-prev {
      display: none;
    }
  }
</style>
