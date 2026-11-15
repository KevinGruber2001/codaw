/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { Command, CommandContribution, CommandRegistry } from '@theia/core/lib/common/command';
import { codicon } from '@theia/core/lib/browser/widgets';
import { ArrangementWidget } from './arrangement-widget.js';

export namespace ArrangementCommands {
    const category = 'Arrangement';
    export const ZOOM_IN: Command = { id: 'codaw.arrangement.zoomIn', category, label: 'Zoom In', iconClass: codicon('zoom-in') };
    export const ZOOM_OUT: Command = { id: 'codaw.arrangement.zoomOut', category, label: 'Zoom Out', iconClass: codicon('zoom-out') };
    export const ZOOM_TO_FIT: Command = { id: 'codaw.arrangement.zoomToFit', category, label: 'Zoom to Fit', iconClass: codicon('screen-full') };
}

/**
 * The arrangement's commands. The arrangement is not a view among others: it is always there, placed by
 * the application shell, so there is no command to open or close it, and there is only ever one.
 */
@injectable()
export class ArrangementContribution implements CommandContribution {

    @inject(ArrangementWidget)
    protected readonly arrangement: ArrangementWidget;

    registerCommands(commands: CommandRegistry): void {
        commands.registerCommand(ArrangementCommands.ZOOM_IN, { execute: () => this.arrangement.zoomBy(1.25) });
        commands.registerCommand(ArrangementCommands.ZOOM_OUT, { execute: () => this.arrangement.zoomBy(0.8) });
        commands.registerCommand(ArrangementCommands.ZOOM_TO_FIT, { execute: () => this.arrangement.zoomToFit() });
    }
}
