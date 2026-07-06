<script lang="ts">
  // The session mixer: every track, bus, and master side by side. Edits route
  // back per-channel to the right TOML file (channel.file) — the mixer is a
  // window over many documents, not a document itself.
  import ChannelStrip from '../components/ChannelStrip.svelte';
  import { send, ready } from '../lib/vscode';
  import type { MixerChannel } from '../../src/protocol';

  let channels = $state<MixerChannel[]>([]);

  ready((msg) => {
    if (msg.type === 'mixer') {
      channels = msg.channels;
    }
  });

  let tracks = $derived(channels.filter((c) => c.kind === 'track'));
  let buses = $derived(channels.filter((c) => c.kind === 'bus'));
  let master = $derived(channels.filter((c) => c.kind === 'master'));

  function edit(channel: MixerChannel, key: string, value: number | boolean) {
    send({ type: 'edit', file: channel.file, key, value });
  }
</script>

<div class="mixer">
  {#each [
    { label: 'tracks', chans: tracks },
    { label: 'buses', chans: buses },
    { label: 'master', chans: master },
  ] as group (group.label)}
    {#if group.chans.length > 0}
      <div class="group">
        <div class="daw-label">{group.label}</div>
        <div class="strips">
          {#each group.chans as channel (channel.file)}
            <ChannelStrip {channel} onedit={(key, value) => edit(channel, key, value)} />
          {/each}
        </div>
      </div>
    {/if}
  {/each}
  {#if channels.length === 0}
    <div class="daw-label loading">loading session…</div>
  {/if}
</div>

<style>
  .mixer {
    display: flex;
    align-items: flex-start;
    gap: 20px;
    padding: 16px;
    overflow-x: auto;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .strips {
    display: flex;
    gap: 6px;
  }
  .loading {
    padding: 16px;
  }
</style>
