/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * Adapted from the Eclipse Theia IDE (theia-extensions/product), which is
 * available under the terms of the MIT License.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { Command, CommandContribution, CommandRegistry } from '@theia/core/lib/common/command';
import { DOCUMENTATION_URL, ISSUES_URL, RELEASES_URL } from './branding-util.js';
import { MenuContribution, MenuModelRegistry, MenuPath } from '@theia/core/lib/common/menu';
import { inject, injectable } from '@theia/core/shared/inversify';

import { CommonMenus } from '@theia/core/lib/browser/common-frontend-contribution';
import { WindowService } from '@theia/core/lib/browser/window/window-service';

export namespace CodawMenus {
    export const CODAW_HELP: MenuPath = [...CommonMenus.HELP, 'codaw'];
}

export namespace CodawCommands {
    export const CATEGORY = 'CodaW';
    export const DOCUMENTATION: Command = {
        id: 'codaw:documentation',
        category: CATEGORY,
        label: 'Documentation'
    };
    export const REPORT_ISSUE: Command = {
        id: 'codaw:report-issue',
        category: CATEGORY,
        label: 'Report Issue'
    };
    export const DOWNLOAD_ENGINE: Command = {
        id: 'codaw:download-engine',
        category: CATEGORY,
        label: 'Download Audio Engine'
    };
}

@injectable()
export class CodawContribution implements CommandContribution, MenuContribution {

    @inject(WindowService)
    protected readonly windowService: WindowService;

    registerCommands(commands: CommandRegistry): void {
        const open = (url: string) => () => this.windowService.openNewWindow(url, { external: true });
        commands.registerCommand(CodawCommands.DOCUMENTATION, { execute: open(DOCUMENTATION_URL) });
        commands.registerCommand(CodawCommands.REPORT_ISSUE, { execute: open(ISSUES_URL) });
        commands.registerCommand(CodawCommands.DOWNLOAD_ENGINE, { execute: open(RELEASES_URL) });
    }

    registerMenus(menus: MenuModelRegistry): void {
        [CodawCommands.DOCUMENTATION, CodawCommands.REPORT_ISSUE, CodawCommands.DOWNLOAD_ENGINE]
            .forEach((command, index) => menus.registerMenuAction(CodawMenus.CODAW_HELP, {
                commandId: command.id,
                label: command.label,
                order: String(index + 1)
            }));
    }
}
