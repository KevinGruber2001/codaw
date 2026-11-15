/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { CommandContribution, CommandRegistry } from '@theia/core/lib/common/command';
import { KeybindingContribution, KeybindingRegistry } from '@theia/core/lib/browser/keybinding';
import { inject, injectable } from '@theia/core/shared/inversify';

import { TransportCommands } from './transport-commands.js';
import { TransportService } from './transport-service.js';

/**
 * Transport keybindings must never fire while the user is typing: Space plays and pauses, but in an
 * editor or an input it has to insert a space.
 */
const NOT_TYPING = '!editorTextFocus && !textInputFocus';

@injectable()
export class TransportContribution implements CommandContribution, KeybindingContribution {

    @inject(TransportService)
    protected readonly transport: TransportService;

    registerCommands(commands: CommandRegistry): void {
        // Only with an engine to send them to.
        const ready = () => this.transport.ready;
        commands.registerCommand(TransportCommands.TOGGLE_PLAY, {
            execute: () => this.transport.togglePlay(),
            isEnabled: ready
        });
        commands.registerCommand(TransportCommands.STOP, {
            execute: () => this.transport.stop(),
            isEnabled: ready
        });
        commands.registerCommand(TransportCommands.RETURN_TO_START, {
            execute: () => this.transport.returnToStart(),
            isEnabled: ready
        });
        commands.registerCommand(TransportCommands.TOGGLE_LOOP, {
            execute: () => this.transport.toggleLoop(),
            // Lets menus and the palette show loop as a toggle with its current state.
            isToggled: () => this.transport.state.looping
        });
    }

    registerKeybindings(keybindings: KeybindingRegistry): void {
        keybindings.registerKeybinding({
            command: TransportCommands.TOGGLE_PLAY.id,
            keybinding: 'space',
            when: NOT_TYPING
        });
        keybindings.registerKeybinding({
            command: TransportCommands.RETURN_TO_START.id,
            keybinding: 'home',
            when: NOT_TYPING
        });
    }
}
