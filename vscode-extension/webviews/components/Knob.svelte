<script lang="ts">
  // Rotary knob: vertical drag to change, double-click to reset. Frequency
  // style params can opt into a logarithmic response via `log`.
  let {
    label,
    value,
    min,
    max,
    step = 0.01,
    log = false,
    format = (v: number) => v.toFixed(2),
    resetTo = null,
    onchange,
  }: {
    label: string;
    value: number;
    min: number;
    max: number;
    step?: number;
    log?: boolean;
    format?: (v: number) => string;
    resetTo?: number | null;
    onchange: (v: number) => void;
  } = $props();

  let dragging = $state(false);
  let dragValue = $state(0);
  let startY = 0;
  let startFrac = 0;

  let shown = $derived(dragging ? dragValue : value);
  let frac = $derived(toFrac(shown));
  // 270° sweep, from -135° (min) to +135° (max).
  let angle = $derived(-135 + frac * 270);

  function toFrac(v: number): number {
    const c = Math.min(max, Math.max(min, v));
    if (log && min > 0) {
      return Math.log(c / min) / Math.log(max / min);
    }
    return (c - min) / (max - min);
  }

  function fromFrac(f: number): number {
    const c = Math.min(1, Math.max(0, f));
    const raw = log && min > 0 ? min * Math.pow(max / min, c) : min + c * (max - min);
    return Math.round(raw / step) * step;
  }

  function onpointerdown(e: PointerEvent) {
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    dragging = true;
    dragValue = value;
    startY = e.clientY;
    startFrac = toFrac(value);
  }

  function onpointermove(e: PointerEvent) {
    if (!dragging) {
      return;
    }
    // 150px of vertical travel = full sweep; fine control without modifiers.
    dragValue = fromFrac(startFrac + (startY - e.clientY) / 150);
  }

  function onpointerup() {
    if (dragging) {
      dragging = false;
      onchange(dragValue);
    }
  }

  function ondblclick() {
    if (resetTo !== null) {
      onchange(resetTo);
    }
  }
</script>

<div class="knob">
  <div
    class="dial"
    role="slider"
    tabindex="0"
    aria-label={label}
    aria-valuemin={min}
    aria-valuemax={max}
    aria-valuenow={shown}
    {onpointerdown}
    {onpointermove}
    {onpointerup}
    {ondblclick}
  >
    <div class="pointer" style="transform: rotate({angle}deg)"></div>
  </div>
  <span class="daw-label">{label}</span>
  <span class="daw-value">{format(shown)}</span>
</div>

<style>
  .knob {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    width: 52px;
  }
  .dial {
    position: relative;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--daw-track);
    border: 1px solid var(--daw-border);
    cursor: ns-resize;
    touch-action: none;
  }
  .pointer {
    position: absolute;
    inset: 0;
  }
  .pointer::after {
    content: '';
    position: absolute;
    top: 2px;
    left: calc(50% - 1px);
    width: 2px;
    height: 10px;
    background: var(--daw-accent);
    border-radius: 1px;
  }
  .dial:focus-visible {
    outline: 1px solid var(--daw-accent);
  }
  .daw-label {
    max-width: 52px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
