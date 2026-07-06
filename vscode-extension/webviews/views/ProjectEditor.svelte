<script lang="ts">
  // project.toml editor: transport settings + the session layout at a glance.
  import Knob from '../components/Knob.svelte';
  import { send, ready } from '../lib/vscode';
  import type { ProjectData } from '../../src/protocol';

  let project = $state<ProjectData | null>(null);

  ready((msg) => {
    if (msg.type === 'update') {
      project = msg.data as ProjectData;
    }
  });
</script>

{#if project}
  <div class="page">
    <header>
      <h1>{project.name}</h1>
      <span class="daw-label">project</span>
    </header>

    <section class="transport daw-card">
      <Knob
        label="bpm"
        value={project.bpm}
        min={20}
        max={300}
        step={1}
        resetTo={120}
        format={(v) => v.toFixed(0)}
        onchange={(v) => send({ type: 'edit', key: 'bpm', value: v })}
      />
      <div class="fact">
        <span class="daw-label">time sig</span>
        <span class="daw-value">{project.timeSig}</span>
      </div>
      <div class="fact">
        <span class="daw-label">sample rate</span>
        <span class="daw-value">{project.sampleRate} Hz</span>
      </div>
      <div class="fact">
        <span class="daw-label">bit depth</span>
        <span class="daw-value">{project.bitDepth} bit</span>
      </div>
    </section>

    <section>
      <div class="daw-label heading">tracks</div>
      {#each project.tracks as file (file)}
        <div class="daw-value row">{file}</div>
      {/each}
    </section>

    <section>
      <div class="daw-label heading">buses</div>
      {#if project.buses.length === 0}
        <div class="daw-label">none</div>
      {/if}
      {#each project.buses as file (file)}
        <div class="daw-value row">{file}</div>
      {/each}
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
  .transport {
    display: flex;
    align-items: center;
    gap: 24px;
    padding: 12px 16px;
    width: fit-content;
  }
  .fact {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .heading {
    margin-bottom: 6px;
  }
  .row {
    padding: 1px 0;
  }
  .loading {
    padding: 16px;
  }
</style>
