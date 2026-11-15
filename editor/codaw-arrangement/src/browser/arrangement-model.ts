/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { CodawProject, ProjectAutomation, ProjectFx } from 'codaw-project/lib/common/project-model';
import { ProjectSchema } from 'codaw-project/lib/common/project-schema';

/*
 * What the arrangement view shows, in the shape of the project's TOML: a track per `tracks/*.toml`, a
 * clip per `[[clip]]`, an automation lane per `[[automation]]`. Positions are in beats, like the files.
 * Built from the project (`fromProject`), plus the view's own state, like which tracks are expanded.
 */

export interface Arrangement {
    bpm: number;
    beatsPerBar: number;
    beatUnit: number;
    /** The transport's loop region, if one is set. */
    loop?: { start: number; end: number };
    tracks: ArrangementTrack[];
}

export interface ArrangementTrack {
    id: string;
    /** The track's file, relative to the project. */
    file: string;
    uri: string;
    /** The bus it feeds; `undefined` is the master. */
    bus?: string;
    /** Index into the track color palette, see `--codaw-track-color-*`. */
    color: number;
    mute: boolean;
    solo: boolean;
    clips: ArrangementClip[];
    automation: AutomationLane[];
    /** Whether the automation lanes are shown under the track. View state, not project data. */
    expanded: boolean;
}

export interface ArrangementClip {
    /** The audio file, relative to the project. */
    file: string;
    uri: string;
    start: number;
    end: number;
    loop: boolean;
    gain: number;
    /** Seconds into the source file where the clip begins. */
    offset: number;
}

export interface AutomationLane {
    /** `gain`, `pan`, or `fx[N].key`, as in the TOML. */
    target: string;
    points: { beat: number; value: number }[];
    /** The range the lane is drawn in. */
    min: number;
    max: number;
    unit: string;
    /** Drawn on a logarithmic scale, as frequencies are heard. */
    log?: boolean;
}

/** A clip, by position: the track's index and the clip's index within the track. */
export interface ClipRef {
    track: number;
    clip: number;
}

export namespace ClipRef {
    export function equals(a: ClipRef | undefined, b: ClipRef | undefined): boolean {
        return !!a && !!b && a.track === b.track && a.clip === b.clip;
    }
}

/** What a clip is being dragged by: its body moves it, its edges trim it. */
export type DragEdge = 'move' | 'start' | 'end';

export interface ClipDrag {
    ref: ClipRef;
    edge: DragEdge;
    pointerStartX: number;
    originalStart: number;
    originalEnd: number;
    /** Where the clip is right now, while dragging. */
    start: number;
    end: number;
}

export type Snap = 'bar' | 'beat' | '1/2' | '1/4' | 'off';

export const SNAP_OPTIONS: { value: Snap; label: string }[] = [
    { value: 'bar', label: 'Bar' },
    { value: 'beat', label: 'Beat' },
    { value: '1/2', label: '1/2 beat' },
    { value: '1/4', label: '1/4 beat' },
    { value: 'off', label: 'Off' }
];

/** The grid size of a snap setting, in beats; 0 for none. */
export function snapStep(snap: Snap, beatsPerBar: number): number {
    switch (snap) {
        case 'bar': return beatsPerBar;
        case 'beat': return 1;
        case '1/2': return 0.5;
        case '1/4': return 0.25;
        default: return 0;
    }
}

/** Rounds a beat to the grid, or to a hundredth of a beat when snapping is off. */
export function snapBeat(beat: number, step: number): number {
    return step > 0 ? Math.round(beat / step) * step : Math.round(beat * 100) / 100;
}

/** The clip's file name without folder and extension: what its label shows. */
export function clipName(clip: ArrangementClip): string {
    const base = clip.file.split('/').pop() ?? clip.file;
    return base.replace(/\.[^.]+$/, '');
}

/** The last beat anything sits at. */
export function arrangementEnd(arrangement: Arrangement): number {
    let end = 0;
    for (const track of arrangement.tracks) {
        for (const clip of track.clips) {
            end = Math.max(end, clip.end);
        }
    }
    return end;
}

/** The value of an automation lane at a beat: linear between points, flat before the first and after the last. */
export function automationValue(lane: AutomationLane, beat: number): number {
    const points = lane.points;
    if (points.length === 0) {
        return 0;
    }
    if (beat <= points[0].beat) {
        return points[0].value;
    }
    for (let i = 1; i < points.length; i++) {
        const a = points[i - 1];
        const b = points[i];
        if (beat <= b.beat) {
            return a.value + (b.value - a.value) * (beat - a.beat) / (b.beat - a.beat);
        }
    }
    return points[points.length - 1].value;
}

/** Where a value sits in its lane's range, from 0 (min) to 1 (max). */
export function automationPosition(lane: AutomationLane, value: number): number {
    if (lane.log && lane.min > 0) {
        return Math.log(value / lane.min) / Math.log(lane.max / lane.min);
    }
    return (value - lane.min) / (lane.max - lane.min);
}

/**
 * The arrangement of a project. `expanded` carries over which tracks had their automation open, by track
 * id, so reading the project again does not fold them all up.
 */
export function fromProject(project: CodawProject, expanded: ReadonlySet<string>, schema?: ProjectSchema): Arrangement {
    return {
        bpm: project.bpm,
        beatsPerBar: project.beatsPerBar,
        beatUnit: project.beatUnit,
        tracks: project.tracks.map((track, index) => ({
            id: track.id,
            file: track.file,
            uri: track.uri,
            bus: track.bus,
            color: index,
            mute: track.mute,
            solo: track.solo,
            expanded: expanded.has(track.id),
            clips: track.clips.map(clip => ({ ...clip })),
            automation: track.automation.map(lane => toLane(lane, track.fx, schema))
        }))
    };
}

/**
 * A lane with the range and unit of what it changes, as the schema has them. Frequencies are drawn on a
 * logarithmic scale. A target the schema does not know (or no schema at all) gets a range that fits its
 * points.
 */
function toLane(automation: ProjectAutomation, effects: readonly ProjectFx[], schema: ProjectSchema | undefined): AutomationLane {
    const { target } = automation;
    const points = automation.points.map(p => ({ ...p }));
    const key = ProjectSchema.target(schema, target, effects);
    if (key?.min !== undefined && key.max !== undefined) {
        return { target, points, min: key.min, max: key.max, unit: key.unit ?? '', log: key.unit === 'Hz' };
    }
    const values = points.map(p => p.value);
    return { target, points, min: Math.min(0, ...values), max: Math.max(1, ...values), unit: key?.unit ?? '' };
}

export function formatBeat(beat: number, beatsPerBar: number): string {
    const bar = Math.floor(beat / beatsPerBar) + 1;
    const inBar = beat - (bar - 1) * beatsPerBar;
    const beatInBar = Math.floor(inBar) + 1;
    const sixteenth = Math.floor((inBar - Math.floor(inBar)) * 4) + 1;
    return `${bar}.${beatInBar}.${sixteenth}`;
}
