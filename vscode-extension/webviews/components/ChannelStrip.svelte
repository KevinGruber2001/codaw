<script lang="ts">
  // One vertical mixer channel, OpenDAW-style: name on top, pan, mute/solo,
  // fader. Tracks get the full set; buses/master omit what they don't have.
  import Fader from './Fader.svelte';
  import Knob from './Knob.svelte';
  import Toggle from './Toggle.svelte';
  import type { MixerChannel } from '../../src/protocol';

  let {
    channel,
    onedit,
  }: {
    channel: MixerChannel;
    onedit: (key: string, value: number | boolean) => void;
  } = $props();
</script>

<div class="strip daw-card" class:master={channel.kind === 'master'}>
  <div class="daw-label name" title={channel.file}>{channel.id}</div>
  {#if channel.kind === 'track'}
    <div class="daw-label route">→ {channel.bus || 'master'}</div>
  {:else}
    <div class="daw-label route">{channel.kind}</div>
  {/if}

  {#if channel.pan !== undefined}
    <Knob
      label="pan"
      value={channel.pan}
      min={-1}
      max={1}
      step={0.01}
      resetTo={0}
      format={(v) => (v === 0 ? 'C' : v < 0 ? `L${Math.round(-v * 100)}` : `R${Math.round(v * 100)}`)}
      onchange={(v) => onedit('pan', v)}
    />
  {/if}

  {#if channel.mute !== undefined}
    <div class="toggles">
      <Toggle label="M" tone="mute" active={channel.mute ?? false} onchange={(v) => onedit('mute', v)} />
      <Toggle label="S" tone="solo" active={channel.solo ?? false} onchange={(v) => onedit('solo', v)} />
    </div>
  {/if}

  <Fader value={channel.gain} onchange={(v) => onedit('gain', v)} />
</div>

<style>
  .strip {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    width: 72px;
    padding: 8px 4px;
    flex: none;
  }
  .strip.master {
    border-color: var(--daw-accent);
  }
  .name {
    color: var(--daw-fg);
    max-width: 64px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .route {
    font-size: 9px;
  }
  .toggles {
    display: flex;
    gap: 4px;
  }
</style>
