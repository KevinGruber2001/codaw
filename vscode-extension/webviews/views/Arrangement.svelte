<script lang="ts">
  // The main screen: tracks as lanes, clips in time, a live playhead.
  //
  // Interaction model:
  //   drag a clip horizontally → on release, one atomic TOML edit (start+end)
  //   click/drag the ruler     → seek the engine playhead
  //   M/S on a lane header     → targeted edit in that track's file
  //   +/- or ctrl+wheel        → zoom (pixels per beat)
  //
  // Rendering is plain DOM — at CodaW's scale (dozens of clips) absolutely
  // positioned divs beat a canvas on every axis that matters here: theming
  // via CSS vars, accessibility, and hit-testing for free.
  import Toggle from '../components/Toggle.svelte';
  import { send, ready } from '../lib/vscode';
  import type { ArrangementData, TransportState } from '../../src/protocol';

  let data = $state<ArrangementData | null>(null);
  let transport = $state<TransportState>({ beat: 0, playing: false, engineRunning: false });
  let pxPerBeat = $state(28);

  const HEADER_W = 140;
  const LANE_H = 56;

  ready((msg) => {
    if (msg.type === 'arrangement') {
      data = msg.data;
    } else if (msg.type === 'transport') {
      transport = msg.state;
    }
  });

  // Timeline extends past the last clip so there's room to drag rightwards.
  let totalBeats = $derived.by(() => {
    let last = 0;
    for (const t of data?.tracks ?? []) {
      for (const c of t.clips) {
        last = Math.max(last, c.end);
      }
    }
    return Math.max(64, Math.ceil(last / 16) * 16 + 16);
  });

  let bars = $derived.by(() => {
    const bpb = data?.beatsPerBar ?? 4;
    const out: { beat: number; n: number }[] = [];
    for (let b = 0, n = 1; b < totalBeats; b += bpb, n++) {
      out.push({ beat: b, n });
    }
    return out;
  });

  // ── clip dragging ──────────────────────────────────────────────────────
  interface Drag {
    trackFile: string;
    clipIndex: number;
    duration: number;
    origStart: number;
    startX: number;
    deltaBeats: number;
    snap: boolean;
  }
  let drag = $state<Drag | null>(null);

  function clipPointerDown(e: PointerEvent, trackFile: string, clipIndex: number, start: number, end: number) {
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    drag = {
      trackFile,
      clipIndex,
      duration: end - start,
      origStart: start,
      startX: e.clientX,
      deltaBeats: 0,
      snap: !e.altKey,
    };
  }

  function clipPointerMove(e: PointerEvent) {
    if (!drag) {
      return;
    }
    drag.deltaBeats = (e.clientX - drag.startX) / pxPerBeat;
    drag.snap = !e.altKey;
  }

  function clipPointerUp() {
    if (!drag) {
      return;
    }
    const start = dragStart(drag);
    if (start !== drag.origStart) {
      // Optimistic update: reflect the new position locally BEFORE the
      // TOML-edit round trip. Without this the clip re-renders from the old
      // data for ~100ms (visible snap-back) until the saved file is re-read
      // and pushed. The authoritative refresh then lands on the same values.
      const track = data?.tracks.find((t) => t.file === drag!.trackFile);
      const clip = track?.clips[drag.clipIndex];
      if (clip) {
        clip.start = start;
        clip.end = start + drag.duration;
      }
      send({
        type: 'clipMove',
        file: drag.trackFile,
        index: drag.clipIndex,
        start,
        end: start + drag.duration,
      });
    }
    drag = null;
  }

  // Where a dragged clip currently sits: snapped to whole beats unless Alt.
  function dragStart(d: Drag): number {
    const raw = d.origStart + d.deltaBeats;
    const snapped = d.snap ? Math.round(raw) : Math.round(raw * 100) / 100;
    return Math.max(0, snapped);
  }

  function clipLeft(trackFile: string, index: number, start: number): number {
    if (drag && drag.trackFile === trackFile && drag.clipIndex === index) {
      return dragStart(drag) * pxPerBeat;
    }
    return start * pxPerBeat;
  }

  // ── ruler seeking ──────────────────────────────────────────────────────
  let scrubbing = $state(false);

  function rulerSeek(e: PointerEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const beat = Math.max(0, (e.clientX - rect.left) / pxPerBeat);
    send({ type: 'transport', action: 'seek', beat: Math.round(beat * 4) / 4 });
  }

  function onwheel(e: WheelEvent) {
    if (!e.ctrlKey && !e.metaKey) {
      return;
    }
    e.preventDefault();
    pxPerBeat = Math.min(120, Math.max(6, pxPerBeat * (e.deltaY < 0 ? 1.15 : 0.87)));
  }

  function basename(p: string): string {
    return p.split('/').pop() ?? p;
  }
</script>

<div class="arrangement" {onwheel}>
  <div class="toolbar">
    <button class="tbtn" title={transport.playing ? 'Pause' : 'Play'} onclick={() => send({ type: 'transport', action: 'toggle' })}>
      {#if transport.playing}❚❚{:else}▶{/if}
    </button>
    <button class="tbtn" title="Stop" onclick={() => send({ type: 'transport', action: 'stop' })}>■</button>
    <span class="daw-value beat">{transport.beat.toFixed(1)}</span>
    <span class="daw-label">{data ? `${data.bpm} bpm` : ''}</span>
    <span class="spacer"></span>
    <button class="tbtn" title="Zoom out" onclick={() => (pxPerBeat = Math.max(6, pxPerBeat * 0.8))}>−</button>
    <button class="tbtn" title="Zoom in" onclick={() => (pxPerBeat = Math.min(120, pxPerBeat * 1.25))}>+</button>
    <span class="daw-label hint">drag clips · alt = fine · ctrl+scroll = zoom</span>
  </div>

  {#if data}
    <div class="scroller">
      <div class="content" style="width: {HEADER_W + totalBeats * pxPerBeat}px">
        <div class="row ruler-row">
          <div class="header daw-label" style="width: {HEADER_W}px"></div>
          <div
            class="ruler"
            style="width: {totalBeats * pxPerBeat}px"
            onpointerdown={(e) => { scrubbing = true; (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId); rulerSeek(e); }}
            onpointermove={(e) => scrubbing && rulerSeek(e)}
            onpointerup={() => (scrubbing = false)}
          >
            {#each bars as bar (bar.beat)}
              <div class="bar" style="left: {bar.beat * pxPerBeat}px">
                <span class="daw-label">{bar.n}</span>
              </div>
            {/each}
          </div>
        </div>

        {#each data.tracks as track (track.file)}
          <div class="row lane-row" style="height: {LANE_H}px">
            <div class="header" style="width: {HEADER_W}px">
              <span class="tname" title={track.file}>{track.id}</span>
              <span class="toggles">
                <Toggle label="M" tone="mute" active={track.mute} onchange={(v) => send({ type: 'edit', file: track.file, key: 'mute', value: v })} />
                <Toggle label="S" tone="solo" active={track.solo} onchange={(v) => send({ type: 'edit', file: track.file, key: 'solo', value: v })} />
              </span>
            </div>
            <div class="lane" style="width: {totalBeats * pxPerBeat}px">
              {#each bars as bar (bar.beat)}
                <div class="gridline" style="left: {bar.beat * pxPerBeat}px"></div>
              {/each}
              {#each track.clips as clip, i (i)}
                <div
                  class="clip"
                  class:dragging={drag?.trackFile === track.file && drag?.clipIndex === i}
                  style="left: {clipLeft(track.file, i, clip.start)}px; width: {Math.max(8, (clip.end - clip.start) * pxPerBeat - 2)}px"
                  title="{clip.file}  {clip.start}–{clip.end}{clip.offset ? `  offset ${clip.offset}s` : ''}"
                  onpointerdown={(e) => clipPointerDown(e, track.file, i, clip.start, clip.end)}
                  onpointermove={clipPointerMove}
                  onpointerup={clipPointerUp}
                >
                  <span class="clip-label">{clip.loop ? '∞ ' : ''}{basename(clip.file)}</span>
                </div>
              {/each}
            </div>
          </div>
        {/each}

        <div
          class="playhead"
          class:live={transport.engineRunning}
          style="left: {HEADER_W + transport.beat * pxPerBeat}px"
        ></div>
      </div>
    </div>
  {:else}
    <div class="daw-label loading">loading session…</div>
  {/if}
</div>

<style>
  .arrangement {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--daw-border);
    flex: none;
  }
  .tbtn {
    width: 30px;
    height: 24px;
    font-size: 12px;
    line-height: 1;
    color: var(--vscode-button-foreground);
    background: var(--vscode-button-background);
    border: none;
    border-radius: var(--daw-radius);
    cursor: pointer;
  }
  .tbtn:hover {
    background: var(--vscode-button-hoverBackground);
  }
  .beat {
    font-size: 13px;
    min-width: 48px;
  }
  .spacer {
    flex: 1;
  }
  .hint {
    font-size: 9px;
  }

  .scroller {
    flex: 1;
    overflow: auto;
  }
  .content {
    position: relative;
  }
  .row {
    display: flex;
    border-bottom: 1px solid var(--daw-border);
  }
  .header {
    position: sticky;
    left: 0;
    z-index: 3;
    flex: none;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    padding: 0 8px;
    background: var(--daw-panel);
    border-right: 1px solid var(--daw-border);
    box-sizing: border-box;
  }
  .tname {
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .toggles {
    display: flex;
    gap: 3px;
    flex: none;
  }

  .ruler-row {
    position: sticky;
    top: 0;
    z-index: 4;
    background: var(--daw-panel);
  }
  .ruler {
    position: relative;
    height: 26px;
    cursor: text;
    flex: none;
  }
  .bar {
    position: absolute;
    top: 0;
    bottom: 0;
    border-left: 1px solid var(--daw-border);
    padding-left: 4px;
    display: flex;
    align-items: center;
  }

  .lane {
    position: relative;
    flex: none;
    background: var(--daw-bg);
  }
  .gridline {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--daw-border);
    opacity: 0.45;
    pointer-events: none;
  }
  .clip {
    position: absolute;
    top: 6px;
    bottom: 6px;
    background: color-mix(in srgb, var(--daw-accent) 22%, var(--daw-panel));
    border: 1px solid var(--daw-accent);
    border-radius: var(--daw-radius);
    cursor: grab;
    overflow: hidden;
    display: flex;
    align-items: center;
    padding: 0 6px;
    touch-action: none;
  }
  .clip.dragging {
    cursor: grabbing;
    opacity: 0.85;
    z-index: 2;
  }
  .clip-label {
    font-size: 10px;
    font-family: var(--daw-mono);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--daw-fg);
  }

  .playhead {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--daw-muted);
    pointer-events: none;
    z-index: 5;
  }
  .playhead.live {
    background: var(--daw-solo);
  }
  .loading {
    padding: 16px;
  }
</style>
