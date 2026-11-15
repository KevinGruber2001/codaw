/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { Command, CommandRegistry } from '@theia/core/lib/common/command';
import { PreferenceService } from '@theia/core/lib/common/preferences';
import { MenuContribution, MenuModelRegistry } from '@theia/core/lib/common/menu';
import { inject, injectable } from '@theia/core/shared/inversify';

import { AbstractViewContribution } from '@theia/core/lib/browser/shell/view-contribution';
import { CodawWelcomeWidget } from './codaw-welcome-widget.js';
import { CommonMenus } from '@theia/core/lib/browser/common-frontend-contribution';
import { FrontendApplicationContribution } from '@theia/core/lib/browser';
import { FrontendApplicationStateService } from '@theia/core/lib/browser/frontend-application-state';
import { SHOW_WELCOME_ON_STARTUP } from './codaw-welcome-preferences.js';
import { ProjectService } from 'codaw-project/lib/browser/project-service';

export namespace CodawWelcomeCommands {
    export const OPEN_WELCOME: Command = {
        id: 'codaw.welcome:open',
        category: 'CodaW',
        label: 'Welcome'
    };
}

@injectable()
export class CodawWelcomeContribution extends AbstractViewContribution<CodawWelcomeWidget>
    implements FrontendApplicationContribution, MenuContribution {

    @inject(FrontendApplicationStateService)
    protected readonly stateService: FrontendApplicationStateService;

    @inject(PreferenceService)
    protected readonly preferenceService: PreferenceService;

    @inject(ProjectService)
    protected readonly projectService: ProjectService;

    constructor() {
        super({
            widgetId: CodawWelcomeWidget.ID,
            widgetName: CodawWelcomeWidget.LABEL,
            defaultWidgetOptions: { area: 'main' }
        });
    }

    async onStart(): Promise<void> {
        // The welcome page is for when there is nothing to work on: no CodaW project open. With one, the
        // arrangement is the start screen, and the main area is the file slot beside it.
        this.stateService.reachedState('ready').then(async () => {
            await this.preferenceService.ready;
            if (!this.preferenceService.get<boolean>(SHOW_WELCOME_ON_STARTUP, true)) {
                return;
            }
            await this.projectService.ready;
            if (!this.projectService.project && this.shell.getWidgets('main').length === 0) {
                this.openView({ reveal: true, activate: true });
            }
        });
    }

    registerCommands(commands: CommandRegistry): void {
        super.registerCommands(commands);
        commands.registerCommand(CodawWelcomeCommands.OPEN_WELCOME, {
            execute: () => this.openView({ reveal: true, activate: true })
        });
    }

    registerMenus(menus: MenuModelRegistry): void {
        super.registerMenus(menus);
        menus.registerMenuAction(CommonMenus.HELP, {
            commandId: CodawWelcomeCommands.OPEN_WELCOME.id,
            label: CodawWelcomeCommands.OPEN_WELCOME.label,
            order: 'a1'
        });
    }
}
