/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution } from '@theia/core/lib/browser/frontend-application-contribution';
import { StatusBar, StatusBarAlignment } from '@theia/core/lib/browser/status-bar/status-bar-types';
import { EngineStatus } from '../common/engine-protocol.js';
import { EngineCommands } from './engine-frontend-contribution.js';
import { EngineService } from './engine-service.js';

const STATUS_BAR_ID = 'codaw-engine-status';

const LABELS: Record<EngineStatus, string> = {
    offline: 'Engine offline',
    starting: 'Engine starting…',
    running: 'Engine live',
    error: 'Engine error'
};

const ICONS: Record<EngineStatus, string> = {
    offline: 'circle-outline',
    starting: 'loading~spin',
    running: 'circle-filled',
    error: 'error'
};

/**
 * The engine's status, at the left end of the status bar, where Theia shows the state of what the
 * application is connected to. The tooltip says why, for an error; a click restarts the engine.
 */
@injectable()
export class EngineStatusBarContribution implements FrontendApplicationContribution {

    @inject(StatusBar)
    protected readonly statusBar: StatusBar;

    @inject(EngineService)
    protected readonly engine: EngineService;

    onStart(): void {
        this.show();
        this.engine.onDidChangeStatus(() => this.show());
    }

    protected show(): void {
        const status = this.engine.status;
        const reason = this.engine.statusMessage;
        this.statusBar.setElement(STATUS_BAR_ID, {
            name: 'Audio Engine',
            text: `$(${ICONS[status]}) ${LABELS[status]}`,
            alignment: StatusBarAlignment.LEFT,
            priority: 1000,   // first on the left
            className: `codaw-engine-status codaw-engine-status-${status}`,
            tooltip: `${reason ?? 'The CodaW audio engine'}. Click to restart it.`,
            command: EngineCommands.RESTART.id
        });
    }
}
