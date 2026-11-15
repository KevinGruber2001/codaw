/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { Command, CommandContribution, CommandRegistry } from '@theia/core/lib/common/command';
import { codicon } from '@theia/core/lib/browser/widgets';
import { KeybindingContribution, KeybindingRegistry } from '@theia/core/lib/browser/keybinding';
import { CommonCommands } from '@theia/core/lib/browser/common-commands';
import { ARRANGEMENT_FOCUS, ArrangementWidget } from './arrangement-widget.js';

export namespace ArrangementCommands {
    const category = 'Arrangement';
    export const ZOOM_IN: Command = { id: 'codaw.arrangement.zoomIn', category, label: 'Zoom In', iconClass: codicon('zoom-in') };
    export const ZOOM_OUT: Command = { id: 'codaw.arrangement.zoomOut', category, label: 'Zoom Out', iconClass: codicon('zoom-out') };
    export const ZOOM_TO_FIT: Command = { id: 'codaw.arrangement.zoomToFit', category, label: 'Zoom to Fit', iconClass: codicon('screen-full') };
    export const COPY_CLIP: Command = { id: 'codaw.arrangement.copyClip', category, label: 'Copy Clip' };
    export const PASTE_CLIP: Command = { id: 'codaw.arrangement.pasteClip', category, label: 'Paste Clip at Playhead' };
    export const DELETE_CLIP: Command = { id: 'codaw.arrangement.deleteClip', category, label: 'Delete Clip' };
}

/**
 * The arrangement's commands. The arrangement is not a view among others: it is always there, placed by
 * the application shell, so there is no command to open or close it, and there is only ever one.
 */
@injectable()
export class ArrangementContribution implements CommandContribution, KeybindingContribution {

    @inject(ArrangementWidget)
    protected readonly arrangement: ArrangementWidget;

    registerCommands(commands: CommandRegistry): void {
        commands.registerCommand(ArrangementCommands.ZOOM_IN, { execute: () => this.arrangement.zoomBy(1.25) });
        commands.registerCommand(ArrangementCommands.ZOOM_OUT, { execute: () => this.arrangement.zoomBy(0.8) });
        commands.registerCommand(ArrangementCommands.ZOOM_TO_FIT, { execute: () => this.arrangement.zoomToFit() });
        commands.registerCommand(ArrangementCommands.COPY_CLIP, {
            execute: () => this.arrangement.copyToClipboard(),
            isEnabled: () => this.arrangement.canCopy
        });
        commands.registerCommand(ArrangementCommands.PASTE_CLIP, {
            execute: () => this.arrangement.paste(),
            isEnabled: () => this.arrangement.canPaste
        });
        commands.registerCommand(ArrangementCommands.DELETE_CLIP, {
            execute: () => this.arrangement.deleteSelection(),
            isEnabled: () => this.arrangement.canCopy
        });

        // Where Theia binds Cmd+C and Cmd+V to its Copy and Paste (in Electron), those take the keys before
        // the arrangement sees them. A command runs its first enabled handler, newest first: these, while the
        // arrangement has the keyboard; Theia's own everywhere else.
        commands.registerHandler(CommonCommands.COPY.id, {
            execute: () => this.arrangement.copyToClipboard(),
            isEnabled: () => this.arrangement.hasFocus && this.arrangement.canCopy
        });
        commands.registerHandler(CommonCommands.PASTE.id, {
            execute: () => this.arrangement.paste(),
            isEnabled: () => this.arrangement.hasFocus && this.arrangement.canPaste
        });
    }

    registerKeybindings(keybindings: KeybindingRegistry): void {
        // Only while the arrangement has the keyboard: in an editor, these keys delete text. Cmd+C and Cmd+V
        // need no binding of their own: see the Copy and Paste handlers above, and the arrangement's keys.
        const when = ARRANGEMENT_FOCUS;
        keybindings.registerKeybinding({ command: ArrangementCommands.DELETE_CLIP.id, keybinding: 'backspace', when });
        keybindings.registerKeybinding({ command: ArrangementCommands.DELETE_CLIP.id, keybinding: 'delete', when });
    }
}
