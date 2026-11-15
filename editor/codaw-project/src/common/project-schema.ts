/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

/**
 * What project files may contain: every section, key and effect, with types, units, ranges and defaults.
 * Not written down here: it is what `codaw schema` prints, from the table the engine validates against
 * (codaw/internal/schema). The views take ranges and units from it instead of knowing them.
 */
export interface ProjectSchema {
    /** Changes when the shape of this object does, not when a key or an effect is added. */
    version: number;
    files: SchemaFile[];
    effects: SchemaEffect[];
}

/** The version of `ProjectSchema` this code reads. */
export const SCHEMA_VERSION = 1;

export type SchemaFileKind = 'project' | 'track' | 'bus' | 'master';

export interface SchemaFile {
    kind: SchemaFileKind;
    sections: SchemaSection[];
}

export interface SchemaSection {
    /** '' for the top of a file, before any header. */
    name: string;
    /** Written `[[name]]`, any number of times. */
    list: boolean;
    doc?: string;
    keys: SchemaKey[];
}

export interface SchemaEffect {
    name: string;
    doc: string;
    params: SchemaKey[];
}

export interface SchemaKey {
    name: string;
    kind: 'number' | 'string' | 'bool' | 'list';
    unit?: string;
    min?: number;
    max?: number;
    /** The only values a number may have. */
    oneOf?: number[];
    suggest?: string[];
    default?: number | string | boolean;
    required?: boolean;
    /** The engine can change it while it plays, without rebuilding anything. */
    live?: boolean;
    doc: string;
}

export namespace ProjectSchema {

    export function is(value: unknown): value is ProjectSchema {
        const schema = value as ProjectSchema | undefined;
        return !!schema && schema.version === SCHEMA_VERSION && Array.isArray(schema.files) && Array.isArray(schema.effects);
    }

    /** A key of a section of a kind of file: `key(schema, 'track', '', 'gain')`, `key(schema, 'track', 'clip', 'start')`. */
    export function key(schema: ProjectSchema | undefined, file: SchemaFileKind, section: string, name: string): SchemaKey | undefined {
        return schema?.files.find(f => f.kind === file)
            ?.sections.find(s => s.name === section)
            ?.keys.find(k => k.name === name);
    }

    export function param(schema: ProjectSchema | undefined, effect: string, name: string): SchemaKey | undefined {
        return schema?.effects.find(e => e.name === effect)?.params.find(p => p.name === name);
    }

    /**
     * What an automation lane's target is, in a track with the given effects: `gain` and `pan` are the
     * track's own keys, `fx[1].wet` a parameter of its second effect.
     */
    export function target(schema: ProjectSchema | undefined, target: string, effects: readonly { type: string }[]): SchemaKey | undefined {
        const fx = /^fx\[(\d+)\]\.(.+)$/.exec(target);
        if (!fx) {
            return key(schema, 'track', '', target);
        }
        const effect = effects[Number(fx[1])];
        return effect && param(schema, effect.type, fx[2]);
    }
}
