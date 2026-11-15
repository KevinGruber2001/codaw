/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

/**
 * Targeted TOML edits: find the one `key = value` line and replace only the value, so an edit from the UI
 * changes exactly what a hand edit would, and comments, key order and alignment stay as they are.
 * Plain functions on text; no file access.
 */

/** Where a key lives: the top of the file (`{}`), a `[section]`, or the `index`-th `[[section]]`. */
export interface TomlTarget {
    section?: string;
    /** Set for an array of tables (`[[clip]]`): which one, counted from 0. */
    index?: number;
}

/** Replace the text from `start` to `end` (character offsets) with `text`. */
export interface TomlEdit {
    start: number;
    end: number;
    text: string;
}

interface Line {
    text: string;
    /** Offset of the line's first character in the whole text. */
    start: number;
}

/**
 * The edit that sets `key` to `value` in `target`: replaces the value if the key is there, otherwise adds
 * a `key = value` line at the end of the section. `undefined` if the section does not exist.
 */
export function valueEdit(text: string, target: TomlTarget, key: string, value: number | boolean | string): TomlEdit | undefined {
    const lines = splitLines(text);
    const bounds = sectionBounds(lines, target);
    if (!bounds) {
        return undefined;
    }
    const formatted = formatTomlValue(value);
    const keyLine = new RegExp(`^(\\s*)${escapeRegExp(key)}(\\s*)=(\\s*)`);

    let lastKeyLine: Line | undefined;
    for (let i = bounds.from; i < bounds.to; i++) {
        const line = lines[i];
        const match = keyLine.exec(line.text);
        if (match) {
            const valueStart = match[0].length;
            const valueEnd = valueStart + valueLength(line.text.slice(valueStart));
            return { start: line.start + valueStart, end: line.start + valueEnd, text: formatted };
        }
        if (/^\s*[A-Za-z0-9_."-]+\s*=/.test(line.text)) {
            lastKeyLine = line;
        }
    }

    // Not there yet: a new line after the section's last key, with `=` where the line above has it.
    const after = lastKeyLine ?? lines[bounds.headerLine ?? -1];
    const column = lastKeyLine ? lastKeyLine.text.indexOf('=') : -1;
    const name = column > key.length ? key.padEnd(column - 1) : key;
    if (!after) {
        // An empty top-level region: at the very start of the file.
        return { start: 0, end: 0, text: `${name} = ${formatted}\n` };
    }
    const end = after.start + after.text.length;
    return { start: end, end, text: `\n${name} = ${formatted}` };
}

/**
 * The lines a target takes up, 0-based and inclusive: from its header to its last line that is not blank
 * (the top-level region: from the first to the last line before the first header). For showing a section.
 */
export function sectionLines(text: string, target: TomlTarget): { first: number; last: number } | undefined {
    const lines = splitLines(text);
    const bounds = sectionBounds(lines, target);
    if (!bounds) {
        return undefined;
    }
    const first = bounds.headerLine ?? bounds.from;
    let last = bounds.to - 1;
    while (last > first && lines[last].text.trim() === '') {
        last--;
    }
    return { first, last: Math.max(first, last) };
}

/**
 * The edit that adds a table to an array of tables: `[[section]]` with the given keys, after the last
 * one there is (after the end of the file if there is none), separated by a blank line. The keys are
 * written in the given order, with their "=" lined up.
 */
export function insertTableEdit(text: string, section: string, values: [string, number | boolean | string][]): TomlEdit {
    const lines = splitLines(text);
    const width = Math.max(...values.map(([key]) => key.length));
    const block = [`[[${section}]]`, ...values.map(([key, value]) => `${key.padEnd(width)} = ${formatTomlValue(value)}`)].join('\n');

    // After the last line with content of the last such table; at the end of the file without one.
    const headers = lines.map((line, i) => ({ i, header: parseHeader(line.text) }))
        .filter(h => h.header?.name === section && h.header.array);
    let after = contentEnd(lines, 0, lines.length);
    if (headers.length > 0) {
        const bounds = sectionBounds(lines, { section, index: headers.length - 1 })!;
        after = contentEnd(lines, bounds.headerLine!, bounds.to);
    }
    const end = lines[after].start + lines[after].text.length;
    return { start: end, end, text: `\n\n${block}` };
}

/**
 * The edit that removes the `index`-th `[[section]]` with everything in it, up to the next table. Its
 * blank lines go with it, so the file does not collect gaps. `undefined` if there is no such table.
 */
export function removeTableEdit(text: string, section: string, index: number): TomlEdit | undefined {
    const lines = splitLines(text);
    const bounds = sectionBounds(lines, { section, index });
    if (!bounds || bounds.headerLine === undefined) {
        return undefined;
    }
    const first = bounds.headerLine;
    const last = contentEnd(lines, first, bounds.to);
    // Its own blank lines after it go too; comments after them stay, as they belong to what follows.
    let next = last + 1;
    while (next < lines.length && lines[next].text.trim() === '') {
        next++;
    }
    if (next >= lines.length) {
        // The last thing in the file: take the blank lines before it instead, so the file ends where the
        // table before it did.
        let before = first;
        while (before > 0 && lines[before - 1].text.trim() === '') {
            before--;
        }
        const start = before > 0 ? lines[before - 1].start + lines[before - 1].text.length : 0;
        return { start, end: text.length, text: before > 0 ? '\n' : '' };
    }
    return { start: lines[first].start, end: lines[next].start, text: '' };
}

/**
 * The last line of a table that is its own: not blank, and not a comment. Comments right above the next
 * header belong to that header ("# Automation: ..." above [[automation]]).
 */
function contentEnd(lines: Line[], from: number, to: number): number {
    let last = to - 1;
    while (last > from && (lines[last].text.trim() === '' || lines[last].text.trim().startsWith('#'))) {
        last--;
    }
    return Math.max(from, last);
}

/** Applies edits computed against the same text. They must not overlap. */
export function applyEdits(text: string, edits: TomlEdit[]): string {
    return [...edits]
        .sort((a, b) => b.start - a.start)
        .reduce((result, edit) => result.slice(0, edit.start) + edit.text + result.slice(edit.end), text);
}

/** A value as TOML writes it. Whole numbers keep a `.0`, like the project files: `20.0`, not `20`. */
export function formatTomlValue(value: number | boolean | string): string {
    if (typeof value === 'number') {
        const rounded = Math.round(value * 1e6) / 1e6;   // no 4.000000000000001 from float arithmetic
        return Number.isInteger(rounded) ? rounded.toFixed(1) : String(rounded);
    }
    if (typeof value === 'boolean') {
        return String(value);
    }
    return JSON.stringify(value);   // a basic TOML string has the same escapes as JSON
}

function splitLines(text: string): Line[] {
    const lines: Line[] = [];
    let start = 0;
    for (const part of text.split('\n')) {
        lines.push({ text: part.replace(/\r$/, ''), start });
        start += part.length + 1;
    }
    return lines;
}

/**
 * The lines of a section, `from` (first line after its header) up to `to`. The top-level region ends at
 * the first header; a section at the next header that is not one of its own sub-tables, so the
 * `[[automation.point]]` tables stay inside their `[[automation]]`.
 */
function sectionBounds(lines: Line[], target: TomlTarget): { from: number; to: number; headerLine?: number } | undefined {
    const headers = lines.map((line, i) => ({ i, header: parseHeader(line.text) })).filter(h => h.header);
    if (target.section === undefined) {
        return { from: 0, to: headers[0]?.i ?? lines.length };
    }
    const isArray = target.index !== undefined;
    const matching = headers.filter(h => h.header!.name === target.section && h.header!.array === isArray);
    const header = matching[target.index ?? 0];
    if (!header) {
        return undefined;
    }
    const prefix = target.section + '.';
    const next = headers.find(h => h.i > header.i && !h.header!.name.startsWith(prefix));
    return { from: header.i + 1, to: next?.i ?? lines.length, headerLine: header.i };
}

function parseHeader(line: string): { name: string; array: boolean } | undefined {
    const match = /^\s*(\[\[?)\s*([^\]]+?)\s*\]\]?\s*(#.*)?$/.exec(line);
    return match ? { name: match[2], array: match[1] === '[[' } : undefined;
}

/** How long the value at the start of `rest` is: up to a comment, without trailing space. */
function valueLength(rest: string): number {
    if (rest.startsWith('"')) {
        // A basic string: up to the closing quote that is not escaped.
        for (let i = 1; i < rest.length; i++) {
            if (rest[i] === '\\') {
                i++;
            } else if (rest[i] === '"') {
                return i + 1;
            }
        }
        return rest.length;
    }
    const comment = rest.indexOf('#');
    return (comment >= 0 ? rest.slice(0, comment) : rest).trimEnd().length;
}

function escapeRegExp(text: string): string {
    return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
