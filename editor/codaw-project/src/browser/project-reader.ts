/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { parse as parseToml } from 'smol-toml';
import URI from '@theia/core/lib/common/uri';
import {
    CodawProject, ProjectAutomation, ProjectBus, ProjectClip, ProjectFx, ProjectMaster, ProjectProblem, ProjectTrack
} from '../common/project-model.js';

/*
 * Turns the text of the project's TOML files into a `CodawProject`. No file access here: the caller reads
 * the files, this decides what they mean. It is lenient where the engine is strict: a missing value gets
 * the engine's default, and a file that does not parse becomes a problem instead of failing the whole
 * project, so the views keep showing everything else while a file is half-edited.
 */

export type Toml = Record<string, unknown>;

/** Parses TOML, or says why it could not. */
export function parseTomlText(text: string): { toml: Toml } | { error: string } {
    try {
        return { toml: parseToml(text) as Toml };
    } catch (error) {
        return { error: error instanceof Error ? error.message.split('\n')[0] : String(error) };
    }
}

/** The member files [layout] names, relative to the project. */
export function layoutFiles(project: Toml): { tracks: string[]; buses: string[]; master?: string } {
    const layout = table(project.layout);
    return {
        tracks: strings(layout.tracks),
        buses: strings(layout.buses),
        master: typeof layout.master === 'string' ? layout.master : undefined
    };
}

export interface MemberFiles {
    tracks: Map<string, Toml>;
    buses: Map<string, Toml>;
    master?: Toml;
}

export function toProject(projectUri: URI, project: Toml, members: MemberFiles, problems: ProjectProblem[]): CodawProject {
    const root = projectUri.parent;
    const meta = table(project.project);
    const transport = table(project.transport);
    const [beatsPerBar, beatUnit] = timeSignature(transport.time_sig);
    const files = layoutFiles(project);

    const tracks: ProjectTrack[] = [];
    for (const file of files.tracks) {
        const toml = members.tracks.get(file);
        if (toml) {
            tracks.push(toTrack(root, file, toml));
        }
    }
    const buses: ProjectBus[] = [];
    for (const file of files.buses) {
        const toml = members.buses.get(file);
        if (toml) {
            buses.push({ file, uri: resolve(root, file), id: str(toml.id, file), gain: num(toml.gain, 0), fx: toFx(toml.fx) });
        }
    }
    let master: ProjectMaster | undefined;
    if (files.master && members.master) {
        const m = members.master;
        master = { file: files.master, uri: resolve(root, files.master), gain: num(m.gain, 0), limiter: m.limiter === true, fx: toFx(m.fx) };
    }

    return {
        uri: projectUri.toString(),
        rootUri: root.toString(),
        name: str(meta.name, root.path.base),
        bpm: num(transport.bpm, 120),
        beatsPerBar,
        beatUnit,
        sampleRate: num(transport.sample_rate, 48000),
        tracks,
        buses,
        master,
        problems
    };
}

function toTrack(root: URI, file: string, toml: Toml): ProjectTrack {
    const bus = typeof toml.bus === 'string' && toml.bus !== '' ? toml.bus : undefined;
    return {
        file,
        uri: resolve(root, file),
        id: str(toml.id, new URI(file).path.name),
        bus,
        gain: num(toml.gain, 0),
        pan: num(toml.pan, 0),
        mute: toml.mute === true,
        solo: toml.solo === true,
        fx: toFx(toml.fx),
        clips: tables(toml.clip).map(clip => toClip(root, clip)),
        automation: tables(toml.automation).map(toAutomation)
    };
}

function toClip(root: URI, clip: Toml): ProjectClip {
    const file = str(clip.file, '');
    return {
        file,
        uri: resolve(root, file),
        start: num(clip.start, 0),
        end: num(clip.end, 0),
        offset: num(clip.offset, 0),
        loop: clip.loop === true,
        gain: num(clip.gain, 0)
    };
}

function toAutomation(automation: Toml): ProjectAutomation {
    return {
        target: str(automation.target, ''),
        points: tables(automation.point)
            .map(p => ({ beat: num(p.beat, 0), value: num(p.value, 0) }))
            .sort((a, b) => a.beat - b.beat)
    };
}

function toFx(value: unknown): ProjectFx[] {
    return tables(value).map(fx => {
        const { type, ...params } = fx;
        return { type: str(type, ''), params };
    });
}

/** "4/4" → [4, 4]. */
function timeSignature(value: unknown): [number, number] {
    const [top, bottom] = String(value ?? '4/4').split('/').map(Number);
    return [top > 0 ? top : 4, bottom > 0 ? bottom : 4];
}

function resolve(root: URI, file: string): string {
    return root.resolve(file).normalizePath().toString();
}

function table(value: unknown): Toml {
    return typeof value === 'object' && value !== null && !Array.isArray(value) ? value as Toml : {};
}

function tables(value: unknown): Toml[] {
    return Array.isArray(value) ? value.filter(v => typeof v === 'object' && v !== null) as Toml[] : [];
}

function strings(value: unknown): string[] {
    return Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : [];
}

function str(value: unknown, fallback: string): string {
    return typeof value === 'string' ? value : fallback;
}

/** TOML integers can come back as bigint; everything here is a plain number. */
function num(value: unknown, fallback: number): number {
    if (typeof value === 'number') {
        return value;
    }
    if (typeof value === 'bigint') {
        return Number(value);
    }
    return fallback;
}
