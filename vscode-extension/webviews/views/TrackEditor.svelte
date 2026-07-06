<script lang="ts">
  // Focused editor for one track file: the "channel strip page". Everything
  // here writes back as a targeted TOML edit — the file stays hand-editable.
  import Fader from '../components/Fader.svelte';
  import Knob from '../components/Knob.svelte';
  import Toggle from '../components/Toggle.svelte';
  import FxChain from '../components/FxChain.svelte';
  import { send, ready } from '../lib/vscode';
  import type { TrackData } from '../../src/protocol';

  let track = $state<TrackData | null>(null);
  let invalid = $state<string | null>(null);

  ready((msg) => {
    if (msg.type === 'update') {
      track = msg.data as TrackData;
      invalid = null;
    } else if (msg.type === 'invalid') {
      invalid = msg.message;
    }
  });

  function edit(key: string, value: number | boolean | string) {
    send({ type: 'edit', key, value });
  }
</script>

{#if track}
  <div class="page">
    <header>
      <h1>{track.id}</h1>
      <span class="daw-label">track → {track.bus || 'master'}</span>
      {#if invalid}<span class="invalid">unsaved TOML is invalid: {invalid}</span>{/if}
    </header>

    <section class="mix daw-card">
      <Fader value={track.gain} onchange={(v) => edit('gain', v)} />
      <Knob
        label="pan"
        value={track.pan}
        min={-1}
        max={1}
        step={0.01}
        resetTo={0}
        format={(v) => (v === 0 ? 'C' : v < 0 ? `L${Math.round(-v * 100)}` : `R${Math.round(v * 100)}`)}
        onchange={(v) => edit('pan', v)}
      />
      <div class="toggles">
        <Toggle label="MUTE" tone="mute" active={track.mute} onchange={(v) => edit('mute', v)} />
        <Toggle label="SOLO" tone="solo" active={track.solo} onchange={(v) => edit('solo', v)} />
      </div>
    </section>

    <section>
      <div class="daw-label heading">effects</div>
      <FxChain fx={track.fx} onedit={(index, key, value) => send({ type: 'fxEdit', index, key, value })} />
    </section>

    <section>
      <div class="daw-label heading">clips</div>
      {#if track.clips.length === 0}
        <div class="daw-label">no clips</div>
      {:else}
        <table>
          <thead>
            <tr><th>file</th><th>start</th><th>end</th><th>offset</th><th>loop</th><th>gain</th></tr>
          </thead>
          <tbody>
            {#each track.clips as clip (clip)}
              <tr>
                <td class="file">{clip.file}</td>
                <td>{clip.start}</td>
                <td>{clip.end}</td>
                <td>{clip.offset}s</td>
                <td>{clip.loop ? 'yes' : ''}</td>
                <td>{clip.gain} dB</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>

    <section>
      <div class="daw-label heading">automation</div>
      {#if track.automation.length === 0}
        <div class="daw-label">none</div>
      {:else}
        {#each track.automation as lane (lane.target)}
          <div class="daw-value lane">
            {lane.target}: {lane.points.map((p) => `${p.beat}→${p.value}`).join('  ')}
          </div>
        {/each}
      {/if}
    </section>
  </div>
{:else}
  <div class="daw-label loading">loading…</div>
{/if}

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 16px;
    max-width: 760px;
  }
  header {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }
  h1 {
    margin: 0;
    font-size: 18px;
    font-weight: 500;
  }
  .invalid {
    color: var(--daw-mute);
    font-size: 11px;
  }
  .mix {
    display: flex;
    align-items: flex-end;
    gap: 20px;
    padding: 12px 16px;
    width: fit-content;
  }
  .toggles {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .heading {
    margin-bottom: 8px;
  }
  table {
    border-collapse: collapse;
    font-size: 11px;
    font-family: var(--daw-mono);
  }
  th {
    text-align: left;
    padding: 2px 12px 2px 0;
    color: var(--daw-muted);
    font-weight: 400;
  }
  td {
    padding: 2px 12px 2px 0;
  }
  .file {
    color: var(--daw-fg);
  }
  .lane {
    padding: 2px 0;
  }
  .loading {
    padding: 16px;
  }
</style>
