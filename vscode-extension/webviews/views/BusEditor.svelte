<script lang="ts">
  import Fader from '../components/Fader.svelte';
  import FxChain from '../components/FxChain.svelte';
  import { send, ready } from '../lib/vscode';
  import type { BusData } from '../../src/protocol';

  let bus = $state<BusData | null>(null);

  ready((msg) => {
    if (msg.type === 'update') {
      bus = msg.data as BusData;
    }
  });
</script>

{#if bus}
  <div class="page">
    <header>
      <h1>{bus.id}</h1>
      <span class="daw-label">bus → master</span>
    </header>

    <section class="mix daw-card">
      <Fader value={bus.gain} onchange={(v) => send({ type: 'edit', key: 'gain', value: v })} />
    </section>

    <section>
      <div class="daw-label heading">effects</div>
      <FxChain fx={bus.fx} onedit={(index, key, value) => send({ type: 'fxEdit', index, key, value })} />
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
