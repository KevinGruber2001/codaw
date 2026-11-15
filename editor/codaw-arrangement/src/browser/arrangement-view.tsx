/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { codicon } from '@theia/core/lib/browser/widgets';
import {
    Arrangement, ArrangementClip, ArrangementTrack, AutomationLane, ClipDrag, ClipRef, DragEdge, SNAP_OPTIONS, Snap,
    arrangementEnd, automationPosition, automationValue, clipName
} from './arrangement-model.js';
import { AudioPeaks } from 'codaw-project/lib/common/audio-peaks';

/*
 * The arrangement, drawn: a ruler along the top, a header and a lane per track, clips in the lanes, the
 * playhead over all of it. Pure rendering; the state and what the gestures do live in `ArrangementWidget`.
 *
 * Plain DOM rather than a canvas: at a song's scale (dozens of clips, not thousands) positioned elements
 * are fast enough, take the theme's colors through CSS variables, and get hit-testing, tooltips and
 * accessibility for free.
 */

export const HEADER_WIDTH = 200;
const LANE_HEIGHT = 64;
const AUTOMATION_HEIGHT = 44;
/** Pixels per drawn peak in a waveform. */
const PX_PER_PEAK = 2;

export interface ArrangementViewProps {
    arrangement: Arrangement;
    pxPerBeat: number;
    snap: Snap;
    playhead: number;
    selection?: ClipRef;
    drag?: ClipDrag;
    onSnapChange(snap: Snap): void;
    onRulerPointerDown(event: React.PointerEvent<HTMLElement>): void;
    onRulerPointerMove(event: React.PointerEvent<HTMLElement>): void;
    onRulerPointerUp(event: React.PointerEvent<HTMLElement>): void;
    onClipPointerDown(event: React.PointerEvent<HTMLElement>, ref: ClipRef, edge: DragEdge): void;
    onClipPointerMove(event: React.PointerEvent<HTMLElement>): void;
    onClipPointerUp(event: React.PointerEvent<HTMLElement>): void;
    onToggleMute(track: number): void;
    onToggleSolo(track: number): void;
    onToggleExpanded(track: number): void;
    onBackgroundPointerDown(): void;
    /** The audio of a clip's file, once it has been read. */
    audioOf(uri: string): AudioPeaks | undefined;
    /** Why a clip's file could not be read, if it could not. */
    audioErrorOf(uri: string): string | undefined;
    /** Whether a file has unsaved changes: shown here, not yet played by the engine. */
    isDirty(uri: string): boolean;
    onZoom(zoom: 'in' | 'out' | 'fit'): void;
    /** Show a track's file. */
    onRevealTrack(track: number): void;
    /** Show one of a track's automation lanes in its file. */
    onRevealAutomation(track: number, lane: number): void;
}

export function ArrangementView(props: ArrangementViewProps): React.ReactElement {
    const { arrangement, pxPerBeat } = props;
    const { beatsPerBar } = arrangement;
    // Room past the last clip, so there is somewhere to drag to: whole 4-bar phrases, at least 16 bars.
    const phrase = beatsPerBar * 4;
    const totalBeats = Math.max(phrase * 4, Math.ceil(arrangementEnd(arrangement) / phrase) * phrase + phrase * 2);
    const width = totalBeats * pxPerBeat;
    const anySolo = arrangement.tracks.some(t => t.solo);

    // The grid is a background, not elements: bar lines, beat lines (once there is room for them), and
    // every other 4-bar phrase shaded, so the song's sections are visible at a glance.
    const barPx = beatsPerBar * pxPerBeat;
    const gridStyle = {
        '--codaw-bar-px': `${barPx}px`,
        '--codaw-beat-px': `${pxPerBeat >= 10 ? pxPerBeat : barPx}px`,
        '--codaw-phrase-px': `${barPx * 8}px`,
        width
    } as React.CSSProperties;

    return <div className='codaw-arr-scroller' onPointerDown={props.onBackgroundPointerDown}>
        <div className='codaw-arr-content' style={{ width: HEADER_WIDTH + width }}>
            <div className='codaw-arr-row codaw-arr-ruler-row'>
                <div className='codaw-arr-header codaw-arr-corner' style={{ width: HEADER_WIDTH }}>
                    <label className='codaw-arr-snap' title='Snap clips and the playhead to the grid. Hold Alt while dragging to ignore it.'>
                        <i className={codicon('magnet')} />
                        <select className='theia-select' value={props.snap}
                            onPointerDown={e => e.stopPropagation()}
                            onChange={e => props.onSnapChange(e.currentTarget.value as Snap)}>
                            {SNAP_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                        </select>
                    </label>
                    <span className='codaw-arr-zoom' onPointerDown={e => e.stopPropagation()}>
                        <button className={`codaw-arr-icon-button ${codicon('zoom-out')}`} title='Zoom Out (Ctrl+scroll)' onClick={() => props.onZoom('out')} />
                        <button className={`codaw-arr-icon-button ${codicon('zoom-in')}`} title='Zoom In (Ctrl+scroll)' onClick={() => props.onZoom('in')} />
                        <button className={`codaw-arr-icon-button ${codicon('screen-full')}`} title='Zoom to Fit' onClick={() => props.onZoom('fit')} />
                    </span>
                </div>
                <Ruler {...props} width={width} totalBeats={totalBeats} />
            </div>

            {arrangement.tracks.map((track, index) => <React.Fragment key={track.file}>
                <TrackRow {...props} track={track} index={index} gridStyle={gridStyle}
                    audible={!track.mute && (!anySolo || track.solo)} />
                {track.expanded && track.automation.map((lane, laneIndex) =>
                    <AutomationRow key={lane.target} lane={lane} track={track} pxPerBeat={pxPerBeat} totalBeats={totalBeats}
                        gridStyle={gridStyle} onReveal={() => props.onRevealAutomation(index, laneIndex)} />
                )}
            </React.Fragment>)}

            <div className='codaw-arr-row codaw-arr-filler'>
                <div className='codaw-arr-header' style={{ width: HEADER_WIDTH }} />
                <div className='codaw-arr-lane codaw-arr-grid' style={gridStyle} />
            </div>

            <div className='codaw-arr-playhead' style={{ left: HEADER_WIDTH + props.playhead * pxPerBeat }} />
        </div>
    </div>;
}

function Ruler(props: ArrangementViewProps & { width: number; totalBeats: number }): React.ReactElement {
    const { arrangement, pxPerBeat } = props;
    const { beatsPerBar, loop } = arrangement;
    const barPx = beatsPerBar * pxPerBeat;
    // Label as many bars as fit: every bar when zoomed in, every 2nd, 4th, 8th... when zoomed out.
    let every = 1;
    while (every * barPx < 36) {
        every *= 2;
    }
    const labels: React.ReactNode[] = [];
    for (let bar = 0; bar * beatsPerBar < props.totalBeats; bar += every) {
        labels.push(<span key={bar} className='codaw-arr-bar-label' style={{ left: bar * barPx }}>{bar + 1}</span>);
    }
    return <div className='codaw-arr-ruler'
        style={{ width: props.width, '--codaw-bar-px': `${barPx}px`, '--codaw-beat-px': `${pxPerBeat >= 10 ? pxPerBeat : barPx}px` } as React.CSSProperties}
        title='Click or drag to move the playhead'
        onPointerDown={props.onRulerPointerDown}
        onPointerMove={props.onRulerPointerMove}
        onPointerUp={props.onRulerPointerUp}>
        {loop && <div className='codaw-arr-loop' title='Loop region'
            style={{ left: loop.start * pxPerBeat, width: (loop.end - loop.start) * pxPerBeat }} />}
        {labels}
        <div className='codaw-arr-playhead-marker' style={{ left: props.playhead * pxPerBeat }} />
    </div>;
}

function TrackRow(props: ArrangementViewProps & {
    track: ArrangementTrack; index: number; audible: boolean; gridStyle: React.CSSProperties
}): React.ReactElement {
    const { track, index, pxPerBeat } = props;
    const trackColor = { '--codaw-track-color': `var(--codaw-track-color-${track.color % 8})` } as React.CSSProperties;
    return <div className={`codaw-arr-row codaw-arr-track${props.audible ? '' : ' codaw-arr-silent'}`}
        style={{ ...trackColor, height: LANE_HEIGHT }}>
        <div className='codaw-arr-header codaw-arr-track-header' style={{ width: HEADER_WIDTH }} title={track.file}>
            <button className={`codaw-arr-icon-button ${codicon(track.expanded ? 'chevron-down' : 'chevron-right')}`}
                style={{ visibility: track.automation.length ? 'visible' : 'hidden' }}
                title={track.expanded ? 'Hide automation' : 'Show automation'}
                onPointerDown={e => e.stopPropagation()}
                onClick={() => props.onToggleExpanded(index)} />
            <div className='codaw-arr-track-names' title={`${track.file}: click to open`}
                onPointerDown={e => e.stopPropagation()} onClick={() => props.onRevealTrack(index)}>
                <span className='codaw-arr-track-name'>
                    {track.id}
                    {props.isDirty(track.uri) && <span className='codaw-arr-track-dirty'
                        title={`${track.file} has unsaved changes: you see them here, the engine plays them once the file is saved`}> ●</span>}
                </span>
                <span className='codaw-arr-track-route'>→ {track.bus ?? 'master'}</span>
            </div>
            <div className='codaw-arr-track-toggles' onPointerDown={e => e.stopPropagation()}>
                <button className={`codaw-arr-toggle codaw-arr-mute${track.mute ? ' codaw-arr-on' : ''}`}
                    title='Mute' aria-pressed={track.mute} onClick={() => props.onToggleMute(index)}>M</button>
                <button className={`codaw-arr-toggle codaw-arr-solo${track.solo ? ' codaw-arr-on' : ''}`}
                    title='Solo' aria-pressed={track.solo} onClick={() => props.onToggleSolo(index)}>S</button>
            </div>
        </div>
        <div className='codaw-arr-lane codaw-arr-grid' style={props.gridStyle}>
            {track.clips.map((clip, clipIndex) => {
                const ref = { track: index, clip: clipIndex };
                const dragging = props.drag && ClipRef.equals(props.drag.ref, ref) ? props.drag : undefined;
                return <Clip key={clipIndex} clip={clip} clipRef={ref} pxPerBeat={pxPerBeat} bpm={props.arrangement.bpm}
                    audio={props.audioOf(clip.uri)} audioError={props.audioErrorOf(clip.uri)}
                    start={dragging?.start ?? clip.start} end={dragging?.end ?? clip.end}
                    selected={ClipRef.equals(props.selection, ref)} dragging={!!dragging}
                    onPointerDown={props.onClipPointerDown} onPointerMove={props.onClipPointerMove}
                    onPointerUp={props.onClipPointerUp} />;
            })}
        </div>
    </div>;
}

function Clip(props: {
    clip: ArrangementClip; clipRef: ClipRef; pxPerBeat: number; bpm: number; start: number; end: number;
    audio?: AudioPeaks; audioError?: string;
    selected: boolean; dragging: boolean;
    onPointerDown: ArrangementViewProps['onClipPointerDown'];
    onPointerMove: ArrangementViewProps['onClipPointerMove'];
    onPointerUp: ArrangementViewProps['onClipPointerUp'];
}): React.ReactElement {
    const { clip, pxPerBeat, start, end } = props;
    const beats = end - start;
    const className = ['codaw-arr-clip', props.selected ? 'codaw-arr-selected' : '', props.dragging ? 'codaw-arr-dragging' : '']
        .filter(Boolean).join(' ');
    const edge = (which: DragEdge) => ({
        onPointerDown: (e: React.PointerEvent<HTMLElement>) => props.onPointerDown(e, props.clipRef, which),
        onPointerMove: props.onPointerMove,
        onPointerUp: props.onPointerUp
    });
    return <div className={className} style={{ left: start * pxPerBeat, width: Math.max(4, beats * pxPerBeat) }}
        title={`${clip.file}\nbeats ${start}–${end}${clip.loop ? ', looped' : ''}${clip.gain ? `, ${clip.gain} dB` : ''}${clip.offset ? `, from ${clip.offset}s` : ''}`}
        {...edge('move')}>
        <div className='codaw-arr-clip-header'>
            {props.audioError && <i className={`${codicon('warning')} codaw-arr-clip-warning`} title={`${clip.file}: ${props.audioError}`} />}
            {clip.loop && <i className={codicon('sync')} />}
            <span className='codaw-arr-clip-name'>{clipName(clip)}</span>
        </div>
        {props.audio
            ? <Waveform clip={clip} audio={props.audio} beats={beats} bpm={props.bpm} width={beats * pxPerBeat} />
            : <div className='codaw-arr-waveform' />}
        <div className='codaw-arr-clip-edge codaw-arr-clip-edge-start' {...edge('start')} />
        <div className='codaw-arr-clip-edge codaw-arr-clip-edge-end' {...edge('end')} />
    </div>;
}

/**
 * The clip's waveform, mirrored around the middle. It follows the audio the way the engine plays it: from
 * `offset` seconds into the file, and for a looped clip, back to the start of the file at its end.
 *
 * Heights are drawn on a square-root scale: on a linear one, a quiet pad is a flat line next to a drum.
 */
function Waveform(props: { clip: ArrangementClip; audio: AudioPeaks; beats: number; bpm: number; width: number }): React.ReactElement {
    const { clip, audio, beats } = props;
    const count = Math.max(2, Math.min(4000, Math.round(props.width / PX_PER_PEAK)));
    const seconds = beats * 60 / props.bpm;
    const step = seconds / count;
    const heights: number[] = [];
    for (let i = 0; i < count; i++) {
        const at = fileTime(clip.offset + i * step, audio.duration, clip.loop);
        heights.push(at === undefined ? 0 : Math.sqrt(AudioPeaks.between(audio, at, at + step)));
    }
    const top = heights.map((h, i) => `${i},${50 - h * 48}`);
    const bottom = heights.map((h, i) => `${i},${50 + h * 48}`).reverse();
    return <svg className='codaw-arr-waveform' viewBox={`0 0 ${count - 1} 100`} preserveAspectRatio='none'>
        <polygon points={[...top, ...bottom].join(' ')} />
    </svg>;
}

/**
 * Where in the audio file a moment of the clip is, in seconds; `undefined` past the end of a clip that
 * does not loop. A looped clip starts at its offset and wraps to the start of the file, as the engine does.
 */
function fileTime(position: number, duration: number, loop: boolean): number | undefined {
    if (position < duration) {
        return position;
    }
    if (!loop || duration <= 0) {
        return undefined;
    }
    return (position - duration) % duration;
}

function AutomationRow(props: {
    lane: AutomationLane; track: ArrangementTrack; pxPerBeat: number; totalBeats: number; gridStyle: React.CSSProperties;
    onReveal(): void;
}): React.ReactElement {
    const { lane, pxPerBeat, totalBeats } = props;
    const height = AUTOMATION_HEIGHT;
    const pad = 6;
    const y = (value: number) => pad + (1 - automationPosition(lane, value)) * (height - pad * 2);
    // Flat from the start to the first point and from the last point to the end, as the engine plays it.
    const beats = [0, ...lane.points.map(p => p.beat), totalBeats];
    const line = beats.map(b => `${b * pxPerBeat},${y(automationValue(lane, b))}`).join(' ');
    const trackColor = { '--codaw-track-color': `var(--codaw-track-color-${props.track.color % 8})` } as React.CSSProperties;
    return <div className='codaw-arr-row codaw-arr-automation' style={{ ...trackColor, height }}>
        <div className='codaw-arr-header codaw-arr-automation-header' style={{ width: HEADER_WIDTH }}
            title={`${props.track.file}: click to open this lane`}
            onPointerDown={e => e.stopPropagation()} onClick={props.onReveal}>
            <span className='codaw-arr-automation-target'>{lane.target}</span>
            <span className='codaw-arr-automation-range'>{lane.min}…{lane.max} {lane.unit}</span>
        </div>
        <div className='codaw-arr-lane codaw-arr-grid' style={props.gridStyle}>
            <svg className='codaw-arr-automation-curve' width={totalBeats * pxPerBeat} height={height}>
                <polyline points={line} />
                {lane.points.map((p, i) => <circle key={i} cx={p.beat * pxPerBeat} cy={y(p.value)} r={3.5}>
                    <title>{`beat ${p.beat}: ${p.value} ${lane.unit}`}</title>
                </circle>)}
            </svg>
        </div>
    </div>;
}
