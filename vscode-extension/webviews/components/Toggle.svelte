<script lang="ts">
  // Small square latch button (mute/solo/limiter). `tone` picks the lit color.
  let {
    label,
    active,
    tone = 'accent',
    onchange,
  }: {
    label: string;
    active: boolean;
    tone?: 'accent' | 'mute' | 'solo';
    onchange: (v: boolean) => void;
  } = $props();

  const colors: Record<string, string> = {
    accent: 'var(--daw-accent)',
    mute: 'var(--daw-mute)',
    solo: 'var(--daw-solo)',
  };
</script>

<button
  class:active
  style="--tone: {colors[tone]}"
  title={label}
  onclick={() => onchange(!active)}
>
  {label}
</button>

<style>
  button {
    min-width: 26px;
    height: 20px;
    padding: 0 6px;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.05em;
    font-family: var(--daw-font);
    color: var(--daw-muted);
    background: var(--daw-track);
    border: 1px solid var(--daw-border);
    border-radius: var(--daw-radius);
    cursor: pointer;
  }
  button.active {
    color: var(--daw-bg);
    background: var(--tone);
    border-color: var(--tone);
  }
  button:focus-visible {
    outline: 1px solid var(--daw-accent);
  }
</style>
