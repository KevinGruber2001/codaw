/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { CodawProject, ProjectFx } from 'codaw-project/lib/common/project-model';
import { ProjectSchema } from 'codaw-project/lib/common/project-schema';

/*
 * The mixer's strips, from the project: one per track, in layout order, then one per bus, then the master.
 * Each strip is one file, and its values are that file's top-level keys.
 */

export type StripKind = 'track' | 'bus' | 'master';

export interface Strip {
    kind: StripKind;
    /** Unique among the strips: `track:kick`, `bus:drums`, `master`. */
    key: string;
    name: string;
    /** The strip's file. */
    uri: string;
    file: string;
    /** Index into the track palette, for tracks. */
    color?: number;
    gain: number;
    /** Tracks only: buses and the master have no pan in the project format. */
    pan?: number;
    mute?: boolean;
    solo?: boolean;
    /** Where the strip's signal goes. */
    output: string;
    fx: ProjectFx[];
}

export function stripsOf(project: CodawProject): Strip[] {
    return [
        ...project.tracks.map((track, index): Strip => ({
            kind: 'track', key: `track:${track.id}`, name: track.id, uri: track.uri, file: track.file, color: index,
            gain: track.gain, pan: track.pan, mute: track.mute, solo: track.solo,
            output: track.bus ?? 'Master', fx: track.fx
        })),
        ...project.buses.map((bus): Strip => ({
            kind: 'bus', key: `bus:${bus.id}`, name: bus.id, uri: bus.uri, file: bus.file,
            gain: bus.gain, output: 'Master', fx: bus.fx
        })),
        ...(project.master ? [{
            kind: 'master', key: 'master', name: 'Master', uri: project.master.uri, file: project.master.file,
            gain: project.master.gain, output: project.master.limiter ? 'Out · limiter' : 'Out', fx: project.master.fx
        } as Strip] : [])
    ];
}

/** The range of a fader, in dB. */
export interface DbRange {
    min: number;
    max: number;
}

/**
 * The range a gain may have: the schema's, for the `gain` of the kind of file the strip is. Without a
 * schema (the engine binary could not be asked) the faders still need a length; this is it.
 */
export function gainRange(schema: ProjectSchema | undefined, kind: StripKind): DbRange {
    const key = ProjectSchema.key(schema, kind, '', 'gain');
    return key?.min !== undefined && key.max !== undefined ? { min: key.min, max: key.max } : { min: -60, max: 6 };
}

/**
 * Where a level sits on a fader, 0 (bottom) to 1 (top). Not linear in dB: the range a mix lives in, around
 * -20 to 0, gets most of the travel, and the quiet end is compressed, as on a hardware fader.
 */
export function faderPosition(db: number, range: DbRange): number {
    const clamped = Math.min(range.max, Math.max(range.min, db));
    return ((clamped - range.min) / (range.max - range.min)) ** 1.5;
}

/** The level at a fader position: the inverse of `faderPosition`. */
export function faderDb(position: number, range: DbRange): number {
    const clamped = Math.min(1, Math.max(0, position));
    return range.min + (range.max - range.min) * clamped ** (1 / 1.5);
}

/** The marks along a fader: its ends, and the usual steps in between. */
export function faderScale(range: DbRange): number[] {
    const steps = [0, -6, -12, -18, -24, -36, -48].filter(db => db > range.min && db < range.max);
    return [range.max, ...steps, range.min];
}

export function formatDb(db: number, range: DbRange): string {
    if (db <= range.min) {
        return '-∞';
    }
    return `${db > 0 ? '+' : ''}${db.toFixed(1)}`;
}
