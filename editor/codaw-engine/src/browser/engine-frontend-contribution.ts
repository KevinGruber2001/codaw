/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { Command, CommandContribution, CommandRegistry } from '@theia/core/lib/common/command';
import { FrontendApplicationContribution } from '@theia/core/lib/browser/frontend-application-contribution';
import { FileService } from '@theia/filesystem/lib/browser/file-service';
import { WorkspaceService } from '@theia/workspace/lib/browser/workspace-service';
import { EngineService } from './engine-service.js';

export namespace EngineCommands {
    export const RESTART: Command = { id: 'codaw.engine.restart', category: 'CodaW', label: 'Restart Engine' };
    export const STOP: Command = { id: 'codaw.engine.stop', category: 'CodaW', label: 'Stop Engine' };
}

/**
 * Starts the engine when the application opens on a CodaW project: a workspace whose root holds a
 * `project.toml`. Opening another folder reloads the window, which runs this again.
 */
@injectable()
export class EngineFrontendContribution implements FrontendApplicationContribution, CommandContribution {

    @inject(WorkspaceService)
    protected readonly workspaceService: WorkspaceService;

    @inject(FileService)
    protected readonly fileService: FileService;

    @inject(EngineService)
    protected readonly engineService: EngineService;

    onStart(): void {
        // Not awaited: onStart must not hold up the application's startup.
        this.startIfProject();
    }

    registerCommands(commands: CommandRegistry): void {
        commands.registerCommand(EngineCommands.RESTART, {
            execute: () => this.startIfProject()
        });
        commands.registerCommand(EngineCommands.STOP, {
            execute: () => this.engineService.stop(),
            isEnabled: () => this.engineService.status === 'running' || this.engineService.status === 'starting'
        });
    }

    protected async startIfProject(): Promise<void> {
        const root = (await this.workspaceService.roots)[0];
        const projectFile = root?.resource.resolve('project.toml');
        if (projectFile && await this.fileService.exists(projectFile)) {
            await this.engineService.start(projectFile.path.fsPath());
        } else {
            this.engineService.onLog('[engine] no project.toml in the workspace, so no engine to start');
        }
    }
}
