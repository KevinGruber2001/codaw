<script lang="ts">
  // The sidebar transport, now Svelte like everything else. Adds the seek bar
  // the old inline-HTML version never had.
  import { send, ready } from '../lib/vscode';
  import type { TransportState } from '../../src/protocol';

  let beat = $state(0);
  let playing = $state(false);
  let engineRunning = $state(false);

  ready((msg) => {
    if (msg.type === 'transport') {
      beat = msg.state.beat;
      playing = msg.state.playing;
      engineRunning = msg.state.engineRunning;
    }
  });

  function seekBy(delta: number) {
    send({ type: 'transport', action: 'seek', beat: Math.max(0, beat + delta) });
  }
</script>

<div class="transport">
  <div class="row">
    <button class="main" title={playing ? 'Pause' : 'Play'} onclick={() => send({ type: 'transport', action: 'toggle' })}>
      {#if playing}❚❚{:else}▶{/if}
    </button>
    <button title="Stop and return to start" onclick={() => send({ type: 'transport', action: 'stop' })}>■</button>
    <button title="Back 4 beats" onclick={() => seekBy(-4)}>«</button>
    <button title="Forward 4 beats" onclick={() => seekBy(4)}>»</button>
    <span class="daw-value beat">{beat.toFixed(1)}</span>
  </div>
  <div class="daw-label status">
    {engineRunning ? (playing ? 'playing' : 'paused') : 'engine off — press play'}
  </div>
</div>

<style>
  .transport {
    padding: 8px 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  button {
    width: 30px;
    height: 26px;
    font-size: 12px;
    line-height: 1;
    color: var(--vscode-button-foreground);
    background: var(--vscode-button-background);
    border: none;
    border-radius: var(--daw-radius);
    cursor: pointer;
  }
  button:hover {
    background: var(--vscode-button-hoverBackground);
  }
  .main {
    width: 38px;
  }
  .beat {
    margin-left: auto;
    font-size: 13px;
  }
</style>
