<script lang="ts">
  import Fader from '../components/Fader.svelte';
  import Toggle from '../components/Toggle.svelte';
  import FxChain from '../components/FxChain.svelte';
  import { send, ready } from '../lib/vscode';
  import type { MasterData } from '../../src/protocol';

  let master = $state<MasterData | null>(null);

  ready((msg) => {
    if (msg.type === 'update') {
      master = msg.data as MasterData;
    }
  });
</script>

{#if master}
  <div class="page">
    <header>
      <h1>master</h1>
      <span class="daw-label">output</span>
    </header>

    <section class="mix daw-card">
      <Fader value={master.gain} onchange={(v) => send({ type: 'edit', key: 'gain', value: v })} />
      <Toggle
        label="LIMITER"
        active={master.limiter}
        onchange={(v) => send({ type: 'edit', key: 'limiter', value: v })}
      />
    </section>

    <section>
      <div class="daw-label heading">effects</div>
      <FxChain fx={master.fx} onedit={(index, key, value) => send({ type: 'fxEdit', index, key, value })} />
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
  .mix {
    display: flex;
    align-items: flex-end;
    gap: 20px;
    padding: 12px 16px;
    width: fit-content;
  }
  .heading {
    margin-bottom: 8px;
  }
  .loading {
    padding: 16px;
  }
</style>
