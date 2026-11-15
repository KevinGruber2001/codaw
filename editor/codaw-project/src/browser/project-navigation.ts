/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { Command, CommandContribution, CommandRegistry } from '@theia/core/lib/common/command';
import URI from '@theia/core/lib/common/uri';
import { EditorManager } from '@theia/editor/lib/browser/editor-manager';
import { MonacoTextModelService } from '@theia/monaco/lib/browser/monaco-text-model-service';
import { TomlTarget, sectionLines } from './toml-edits.js';

export namespace ProjectCommands {
    /**
     * Opens a project file next to the arrangement, at a section: `(uri: string, target?: TomlTarget)`.
     * What the views run when something in them is clicked: a track opens its file, a clip its `[[clip]]`.
     */
    export const REVEAL_IN_FILE: Command = { id: 'codaw.project.revealInFile' };
}

/**
 * Showing where something is defined. A command rather than a service for the views to inject, so a view
 * does not depend on the editor (and through it on the shell, which holds the arrangement).
 */
@injectable()
export class ProjectNavigationContribution implements CommandContribution {

    @inject(EditorManager)
    protected readonly editorManager: EditorManager;

    @inject(MonacoTextModelService)
    protected readonly textModels: MonacoTextModelService;

    registerCommands(commands: CommandRegistry): void {
        commands.registerCommand(ProjectCommands.REVEAL_IN_FILE, {
            execute: (uri: string, target?: TomlTarget) => this.reveal(uri, target)
        });
    }

    protected async reveal(uri: string, target?: TomlTarget): Promise<void> {
        const lines = target ? await this.linesOf(uri, target) : undefined;
        await this.editorManager.open(new URI(uri), {
            // Shown, not focused: the keyboard stays with the arrangement, where Space plays.
            mode: 'reveal',
            revealOption: 'centerIfOutsideViewport',
            // The section selected, so it stands out; without one, the top of the file.
            selection: lines
                ? { start: { line: lines.first, character: 0 }, end: { line: lines.last + 1, character: 0 } }
                : { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }
        });
    }

    protected async linesOf(uri: string, target: TomlTarget): Promise<{ first: number; last: number } | undefined> {
        const reference = await this.textModels.createModelReference(new URI(uri));
        try {
            return sectionLines(reference.object.textEditorModel.getValue(), target);
        } finally {
            reference.dispose();
        }
    }
}
