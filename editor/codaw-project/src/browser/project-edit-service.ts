/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { MessageService } from '@theia/core/lib/common/message-service';
import URI from '@theia/core/lib/common/uri';
import * as monaco from '@theia/monaco-editor-core';
import { MonacoTextModelService } from '@theia/monaco/lib/browser/monaco-text-model-service';
import { TomlEdit, TomlTarget, insertTableEdit, removeTableEdit, valueEdit } from './toml-edits.js';

/** A clip to add, with every key its [[clip]] gets. */
export interface NewClip {
    file: string;
    start: number;
    end: number;
    loop: boolean;
    gain: number;
    offset: number;
}

interface ValueChange {
    target: TomlTarget;
    key: string;
    value: number | boolean | string;
}

/**
 * Changes to the project's files, made by the views: a clip moved in the arrangement, a track muted.
 *
 * Edits go into the file's text model, the same document an editor tab shows, never straight to disk.
 * So typing in the file and dragging in the arrangement are two edits to one document, there is nothing to
 * merge, and both are on the same undo stack. A document without unsaved changes is saved right away, so
 * the engine (which reads the disk) plays the change; one with unsaved changes is left unsaved, so
 * half-typed text never reaches the engine. The change is then heard on the next save.
 */
@injectable()
export class ProjectEditService {

    @inject(MonacoTextModelService)
    protected readonly textModels: MonacoTextModelService;

    @inject(MessageService)
    protected readonly messages: MessageService;

    /** A clip's new start and end: a move, or a trim that leaves the audio's in-point alone. */
    moveClip(trackUri: string, clip: number, start: number, end: number): Promise<boolean> {
        const target = { section: 'clip', index: clip };
        return this.edit(trackUri, [
            { target, key: 'start', value: start },
            { target, key: 'end', value: end }
        ]);
    }

    /** Trimming a clip's start so the audio stays in place: its in-point moves with it. */
    trimClip(trackUri: string, clip: number, start: number, end: number, offset: number): Promise<boolean> {
        const target = { section: 'clip', index: clip };
        return this.edit(trackUri, [
            { target, key: 'start', value: start },
            { target, key: 'end', value: end },
            { target, key: 'offset', value: offset }
        ]);
    }

    /** The level of a track, bus or master: the top-level `gain` of its file, in dB. */
    setGain(uri: string, gain: number): Promise<boolean> {
        return this.edit(uri, [{ target: {}, key: 'gain', value: Math.round(gain * 10) / 10 }]);
    }

    /** A track's position in the stereo field, -1 (left) to 1 (right). */
    setPan(trackUri: string, pan: number): Promise<boolean> {
        return this.edit(trackUri, [{ target: {}, key: 'pan', value: Math.round(pan * 100) / 100 }]);
    }

    /** Adds a clip to a track: a new [[clip]], after the track's last one. */
    addClip(trackUri: string, clip: NewClip): Promise<boolean> {
        return this.applyEdits(trackUri, text => [insertTableEdit(text, 'clip', [
            ['file', clip.file],
            ['start', clip.start],
            ['end', clip.end],
            ['loop', clip.loop],
            ['gain', clip.gain],
            ['offset', clip.offset]
        ])]);
    }

    /** Removes a track's index-th clip, its [[clip]] table and everything in it. */
    removeClip(trackUri: string, index: number): Promise<boolean> {
        return this.applyEdits(trackUri, text => {
            const edit = removeTableEdit(text, 'clip', index);
            return edit ? [edit] : `it has no [[clip]] #${index + 1} anymore`;
        });
    }

    setMute(trackUri: string, mute: boolean): Promise<boolean> {
        return this.edit(trackUri, [{ target: {}, key: 'mute', value: mute }]);
    }

    setSolo(trackUri: string, solo: boolean): Promise<boolean> {
        return this.edit(trackUri, [{ target: {}, key: 'solo', value: solo }]);
    }

    /**
     * Applies several value changes to one file as one edit: one step to undo, and one save, so the engine
     * never loads a half-done change (a clip with its new start and its old end).
     */
    protected edit(uri: string, changes: ValueChange[]): Promise<boolean> {
        return this.applyEdits(uri, text => {
            const edits: TomlEdit[] = [];
            for (const change of changes) {
                const edit = valueEdit(text, change.target, change.key, change.value);
                if (!edit) {
                    // The file no longer has what the view showed, e.g. the clip was deleted in the editor.
                    return `it has no ${describe(change.target)} anymore`;
                }
                edits.push(edit);
            }
            return edits;
        });
    }

    /**
     * Applies edits computed from a file's current text, as one change: one step to undo, one save. The
     * edits, or why the file cannot be changed as asked, come from `compute`.
     */
    protected async applyEdits(uri: string, compute: (text: string) => TomlEdit[] | string): Promise<boolean> {
        const reference = await this.textModels.createModelReference(new URI(uri));
        try {
            const model = reference.object;
            if (model.readOnly) {
                this.messages.warn(`${new URI(uri).path.base} is read-only.`);
                return false;
            }
            const edits = compute(model.textEditorModel.getValue());
            if (typeof edits === 'string') {
                this.messages.warn(`Could not change ${new URI(uri).path.base}: ${edits}.`);
                return false;
            }

            const wasDirty = model.dirty;
            const textModel = model.textEditorModel;
            const operations = edits.map(edit => ({
                range: monaco.Range.fromPositions(textModel.getPositionAt(edit.start), textModel.getPositionAt(edit.end)),
                text: edit.text
            }));
            // Theia opens an editor for any document that becomes dirty without one, so unsaved changes are
            // never invisible. A clean document is saved right below, so that would only flash a tab open.
            const suppressed = model.suppressOpenEditorWhenDirty;
            model.suppressOpenEditorWhenDirty = suppressed || !wasDirty;
            try {
                // Undo stops on both sides, so the change is one step of its own on the document's undo stack.
                textModel.pushStackElement();
                textModel.pushEditOperations([], operations, () => null);
                textModel.pushStackElement();

                if (!wasDirty) {
                    await model.save();
                }
            } finally {
                model.suppressOpenEditorWhenDirty = suppressed;
            }
            return true;
        } finally {
            reference.dispose();
        }
    }
}

function describe(target: TomlTarget): string {
    if (target.section === undefined) {
        return 'top-level section';
    }
    return target.index === undefined ? `[${target.section}]` : `[[${target.section}]] #${target.index + 1}`;
}
