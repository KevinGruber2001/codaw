<script lang="ts">
  // Vertical dB fader. Dragging updates the readout live but only calls
  // onchange on release — one gesture, one TOML edit, one clean git diff.
  let {
    value,
    min = -60,
    max = 6,
    onchange,
  }: {
    value: number;
    min?: number;
    max?: number;
    onchange: (v: number) => void;
  } = $props();

  const HEIGHT = 120;

  let dragging = $state(false);
  let dragValue = $state(0);

  let shown = $derived(dragging ? dragValue : value);
  let frac = $derived((clamp(shown) - min) / (max - min));

  function clamp(v: number): number {
    return Math.min(max, Math.max(min, v));
  }

  function onpointerdown(e: PointerEvent) {
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    dragging = true;
    dragValue = value;
    moveTo(e);
  }

  function onpointermove(e: PointerEvent) {
    if (dragging) {
      moveTo(e);
    }
  }

  function onpointerup() {
    if (dragging) {
      dragging = false;
      onchange(Math.round(dragValue * 10) / 10);
    }
  }

  function moveTo(e: PointerEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const f = 1 - (e.clientY - rect.top) / rect.height;
    dragValue = clamp(min + f * (max - min));
  }

  function ondblclick() {
    onchange(0); // unity gain
  }
</script>

<div class="fader">
  <div
    class="rail"
    style="height: {HEIGHT}px"
    role="slider"
    tabindex="0"
    aria-valuemin={min}
    aria-valuemax={max}
    aria-valuenow={shown}
    {onpointerdown}
    {onpointermove}
    {onpointerup}
    {ondblclick}
  >
    <div class="fill" style="height: {frac * 100}%"></div>
    <div class="thumb" style="bottom: calc({frac * 100}% - 4px)"></div>
  </div>
  <span class="daw-value">{shown.toFixed(1)}</span>
</div>

<style>
  .fader {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }
  .rail {
    position: relative;
    width: 26px;
    background: var(--daw-track);
    border: 1px solid var(--daw-border);
    border-radius: var(--daw-radius);
    cursor: ns-resize;
    touch-action: none;
  }
  .fill {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    background: color-mix(in srgb, var(--daw-level) 30%, transparent);
  }
  .thumb {
    position: absolute;
    left: -2px;
    right: -2px;
    height: 8px;
    background: var(--daw-fg);
    border-radius: 2px;
  }
  .rail:focus-visible {
    outline: 1px solid var(--daw-accent);
  }
</style>
