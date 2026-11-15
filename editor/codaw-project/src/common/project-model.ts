/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

/*
 * A CodaW project as the editor sees it: what project.toml and the files its [layout] names say, in the
 * shape of the engine's own model (codaw/internal/project/project.go). Plain data, so it can be handed
 * to any view.
 */

export interface CodawProject {
    /** project.toml, as a URI string. */
    uri: string;
    /** The folder holding project.toml; every path in the project is relative to it. */
    rootUri: string;
    name: string;
    bpm: number;
    beatsPerBar: number;
    beatUnit: number;
    sampleRate: number;
    /** In layout order, which is the order the views show them in. */
    tracks: ProjectTrack[];
    buses: ProjectBus[];
    master?: ProjectMaster;
    /** What could not be read. The rest of the project is still there. */
    problems: ProjectProblem[];
}

export interface ProjectTrack {
    /** The track's file, relative to the project, as written in [layout]. */
    file: string;
    uri: string;
    id: string;
    /** The bus it feeds; `undefined` is the master. */
    bus?: string;
    gain: number;
    pan: number;
    mute: boolean;
    solo: boolean;
    fx: ProjectFx[];
    clips: ProjectClip[];
    automation: ProjectAutomation[];
}

export interface ProjectClip {
    /** The audio file, relative to the project, as written. */
    file: string;
    uri: string;
    start: number;
    end: number;
    /** Seconds into the audio file where the clip begins. */
    offset: number;
    loop: boolean;
    gain: number;
}

export interface ProjectAutomation {
    /** `gain`, `pan`, or `fx[N].key`. */
    target: string;
    points: { beat: number; value: number }[];
}

export interface ProjectFx {
    type: string;
    params: Record<string, unknown>;
}

export interface ProjectBus {
    file: string;
    uri: string;
    id: string;
    gain: number;
    fx: ProjectFx[];
}

export interface ProjectMaster {
    file: string;
    uri: string;
    gain: number;
    limiter: boolean;
    fx: ProjectFx[];
}

export interface ProjectProblem {
    /** The file, relative to the project. */
    file: string;
    message: string;
}
