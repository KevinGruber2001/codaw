<script lang="ts">
  // Renders an FX chain as a row of units, each unit a card of knobs — one
  // knob per numeric parameter. The schema is open-ended, so this is fully
  // generic: any effect type renders, with ranges guessed from param names
  // (see lib/fxParams). Editing emits (index, key, value) for a targeted
  // [[fx]] TOML edit on the host side.
  import Knob from './Knob.svelte';
  import { paramSpec, formatParam } from '../lib/fxParams';
  import type { FxData } from '../../src/protocol';

  let {
    fx,
    onedit,
  }: {
    fx: FxData[];
    onedit: (index: number, key: string, value: number) => void;
  } = $props();
</script>

{#if fx.length === 0}
  <div class="daw-label empty">no effects</div>
{:else}
  <div class="chain">
    {#each fx as unit, i (i)}
      <div class="unit daw-card">
        <div class="daw-label name">{unit.type}</div>
        <div class="params">
          {#each Object.entries(unit.params) as [key, value] (key)}
            {@const spec = paramSpec(key)}
            <Knob
              label={key}
              {value}
              min={spec.min}
              max={spec.max}
              step={spec.step}
              log={spec.log ?? false}
              format={(v) => formatParam(v, spec)}
              onchange={(v) => onedit(i, key, v)}
            />
          {/each}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .chain {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .unit {
    padding: 8px;
  }
  .name {
    margin-bottom: 6px;
  }
  .params {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .empty {
    padding: 4px 0;
  }
</style>
