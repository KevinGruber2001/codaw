/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { injectable } from '@theia/core/shared/inversify';
import { ContributionFilterRegistry, FilterContribution } from '@theia/core/lib/common/contribution-filter';
import { MAIN_MENU_BAR, MenuContribution, MenuModelRegistry } from '@theia/core/lib/common/menu';
import { OutlineViewContribution } from '@theia/outline-view/lib/browser/outline-view-contribution';
import { ProblemContribution } from '@theia/markers/lib/browser/problem/problem-contribution';

/**
 * What CodaW leaves out of the Theia it is built on. Packages the app does not need are simply not in
 * its dependencies (the terminal, for one); the ones below come in as dependencies of the editor itself
 * (`@theia/monaco` needs both), so they are in the app either way, and are filtered out here instead.
 *
 * Filtering a contribution removes everything it would have added: its view, its commands, its entries
 * in the View menu, its keybindings and its status bar items.
 */
@injectable()
export class CodawTrimContribution implements FilterContribution, MenuContribution {

    registerContributionFilters(registry: ContributionFilterRegistry): void {
        registry.addFilters('*', [
            // Outline: a code editor's view of symbols in a file. A TOML file has nothing to outline.
            contribution => !(contribution instanceof OutlineViewContribution),
            // Problems, and its counter in the status bar: CodaW is to catch errors in the project where
            // they happen (the arrangement shows broken files; later the TOML language support).
            contribution => !(contribution instanceof ProblemContribution)
        ]);
    }

    registerMenus(menus: MenuModelRegistry): void {
        // A code editor's menus. Everything in them is still in the command palette and keeps its keys.
        menus.unregisterMenuAction('3_selection', MAIN_MENU_BAR);   // Selection (@theia/monaco)
        menus.unregisterMenuAction('5_go', MAIN_MENU_BAR);          // Go (@theia/editor)
    }
}
