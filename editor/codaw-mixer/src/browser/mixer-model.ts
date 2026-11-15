/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { CodawProject, ProjectFx } from 'codaw-project/lib/common/project-model';

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

/** The range the engine accepts for every gain (validate.go). */
export const MIN_DB = -60;
export const MAX_DB = 6;

/**
 * Where a level sits on a fader, 0 (bottom) to 1 (top). Not linear in dB: the range a mix lives in, around
 * -20 to 0, gets most of the travel, and the quiet end is compressed, as on a hardware fader.
 */
export function faderPosition(db: number): number {
    const clamped = Math.min(MAX_DB, Math.max(MIN_DB, db));
    return ((clamped - MIN_DB) / (MAX_DB - MIN_DB)) ** 1.5;
}

/** The level at a fader position: the inverse of `faderPosition`. */
export function faderDb(position: number): number {
    const clamped = Math.min(1, Math.max(0, position));
    return MIN_DB + (MAX_DB - MIN_DB) * clamped ** (1 / 1.5);
}

/** The marks along a fader. */
export const FADER_SCALE = [6, 0, -6, -12, -18, -24, -36, -48, -60];

export function formatDb(db: number): string {
    if (db <= MIN_DB) {
        return '-∞';
    }
    return `${db > 0 ? '+' : ''}${db.toFixed(1)}`;
}
