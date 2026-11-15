/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';

import { CommandRegistry, PreferenceService, environment, isOSX } from '@theia/core/lib/common';
import { LabelProvider, ReactWidget, codicon } from '@theia/core/lib/browser';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';

import { ApplicationInfo, ApplicationServer } from '@theia/core/lib/common/application-protocol';
import { CommonCommands } from '@theia/core/lib/browser/common-frontend-contribution';
import { KeymapsCommands } from '@theia/keymaps/lib/browser/keymaps-frontend-contribution';
import { SHOW_WELCOME_ON_STARTUP } from './codaw-welcome-preferences.js';
import URI from '@theia/core/lib/common/uri';
import { WindowService } from '@theia/core/lib/browser/window/window-service';
import { WorkspaceCommands } from '@theia/workspace/lib/browser/workspace-commands';
import { WorkspaceService } from '@theia/workspace/lib/browser';
import { renderBrandingSections, renderProductName } from './branding-util.js';

/**
 * CodaW's welcome page.
 *
 * It does deliberately not extend Theia's `GettingStartedWidget`: `@theia/getting-started` depends on
 * `@theia/plugin-ext` for its walkthroughs, and that one dependency pulls the VS Code plugin host, the
 * debugger, notebooks and the AI packages into the application — eighteen extensions this product has no
 * use for. Everything the page needs is a handful of commands and the workspace service.
 */
@injectable()
export class CodawWelcomeWidget extends ReactWidget {

    static readonly ID = 'codaw.welcome';
    static readonly LABEL = 'Welcome';

    /** How many recently opened workspaces the page lists. */
    protected static readonly RECENT_LIMIT = 5;

    @inject(ApplicationServer)
    protected readonly applicationServer: ApplicationServer;

    @inject(CommandRegistry)
    protected readonly commands: CommandRegistry;

    @inject(LabelProvider)
    protected readonly labelProvider: LabelProvider;

    @inject(PreferenceService)
    protected readonly preferences: PreferenceService;

    @inject(WindowService)
    protected readonly windowService: WindowService;

    @inject(WorkspaceService)
    protected readonly workspaceService: WorkspaceService;

    protected applicationInfo: ApplicationInfo | undefined;
    protected recentWorkspaces: string[] = [];

    @postConstruct()
    protected init(): void {
        this.id = CodawWelcomeWidget.ID;
        this.title.label = CodawWelcomeWidget.LABEL;
        this.title.caption = CodawWelcomeWidget.LABEL;
        this.title.closable = true;
        // Brand the tab with the application icon, the way a favicon marks a page.
        this.title.iconClass = 'codaw-welcome-tab-icon';
        this.addClass('codaw-welcome');
        this.doInit();
    }

    protected async doInit(): Promise<void> {
        this.applicationInfo = await this.applicationServer.getApplicationInfo();
        this.recentWorkspaces = await this.workspaceService.recentWorkspaces();
        await this.preferences.ready;
        this.update();
    }

    protected render(): React.ReactNode {
        return <div className='codaw-welcome-container'>
            <div className='codaw-welcome-content'>
                {this.renderHeader()}
                <hr className='codaw-rule' />
                <div className='codaw-welcome-grid'>
                    <div className='codaw-welcome-column'>
                        {this.renderStart()}
                        {this.renderRecent()}
                        {this.renderSettings()}
                        {this.renderHelp()}
                    </div>
                    <div className='codaw-welcome-column'>
                        {renderBrandingSections({ windowService: this.windowService })}
                    </div>
                </div>
            </div>
            {this.renderStartupPreference()}
        </div>;
    }

    /**
     * The square application icon rather than a wordmark: it carries the brand without repeating the
     * product name that is set beside it.
     */
    protected renderHeader(): React.ReactNode {
        return <div className='codaw-welcome-header'>
            <div className='theia-icon codaw-welcome-icon'></div>
            <div>
                {renderProductName()}
                <p className='codaw-subtitle'>
                    {'Version ' + (this.applicationInfo?.version ?? '-')}
                </p>
            </div>
        </div>;
    }

    protected renderStart(): React.ReactNode {
        // Only Electron on Windows and Linux can tell a file dialog to pick either a file or a folder;
        // everywhere else a single "Open" has to cover both.
        const singleOpen = isOSX || !environment.electron.is();
        return this.renderSection('files', 'Start', [
            this.renderCommand(CommonCommands.PICK_NEW_FILE.id, 'New File...'),
            ...(singleOpen
                ? [this.renderCommand(WorkspaceCommands.OPEN.id, 'Open...')]
                : [
                    this.renderCommand(WorkspaceCommands.OPEN_FILE.id, 'Open File...'),
                    this.renderCommand(WorkspaceCommands.OPEN_FOLDER.id, 'Open Folder...')
                ]),
            this.renderCommand(WorkspaceCommands.OPEN_WORKSPACE.id, 'Open Workspace...')
        ]);
    }

    protected renderRecent(): React.ReactNode {
        if (this.recentWorkspaces.length === 0) {
            return this.renderSection('history', 'Recent', [
                <span className='codaw-empty' key='empty'>No recent projects yet.</span>
            ]);
        }
        const entries = this.recentWorkspaces.slice(0, CodawWelcomeWidget.RECENT_LIMIT).map(value => {
            const uri = new URI(value);
            return <div className='codaw-action' key={value}>
                {this.renderLink(this.labelProvider.getName(uri), () => this.workspaceService.open(uri))}
                <span className='codaw-path' title={uri.path.fsPath()}>
                    {this.labelProvider.getLongName(uri.parent)}
                </span>
            </div>;
        });
        if (this.recentWorkspaces.length > CodawWelcomeWidget.RECENT_LIMIT) {
            entries.push(<div className='codaw-action' key='more'>
                {this.renderLink('More...', () => this.commands.executeCommand(WorkspaceCommands.OPEN_RECENT_WORKSPACE.id))}
            </div>);
        }
        return this.renderSection('history', 'Recent', entries);
    }

    protected renderSettings(): React.ReactNode {
        return this.renderSection('settings-gear', 'Settings', [
            this.renderCommand(CommonCommands.OPEN_PREFERENCES.id, 'Open Settings'),
            this.renderCommand(KeymapsCommands.OPEN_KEYMAPS.id, 'Open Keyboard Shortcuts')
        ]);
    }

    protected renderHelp(): React.ReactNode {
        const external = (text: string, url: string) => <div className='codaw-action' key={url}>
            {this.renderLink(text, () => this.windowService.openNewWindow(url, { external: true }), text)}
        </div>;
        return this.renderSection('question', 'Help', [
            external('Documentation', 'https://github.com/KevinGruber2001/codaw#readme'),
            external('Source code', 'https://github.com/KevinGruber2001/codaw'),
            external('Report an issue', 'https://github.com/KevinGruber2001/codaw/issues/new')
        ]);
    }

    protected renderStartupPreference(): React.ReactNode {
        const checked = this.preferences.get<boolean>(SHOW_WELCOME_ON_STARTUP, true);
        return <div className='codaw-welcome-footer'>
            <label>
                <input
                    type='checkbox'
                    className='theia-input'
                    checked={checked}
                    onChange={event => {
                        this.preferences.updateValue(SHOW_WELCOME_ON_STARTUP, event.target.checked);
                        this.update();
                    }}
                />
                Show welcome page on startup
            </label>
        </div>;
    }

    protected renderSection(icon: string, title: string, children: React.ReactNode[]): React.ReactNode {
        return <div className='codaw-section' key={title}>
            <h3 className='codaw-section-header'>
                <i className={codicon(icon)}></i>
                {title}
            </h3>
            {children}
        </div>;
    }

    protected renderCommand(commandId: string, label: string): React.ReactNode {
        return <div className='codaw-action' key={commandId}>
            {this.renderLink(label, () => this.commands.executeCommand(commandId), commandId)}
        </div>;
    }

    /**
     * Links are buttons: they run a command rather than navigate, and have to be reachable by keyboard.
     */
    protected renderLink(label: string, run: () => unknown, key?: string): React.ReactNode {
        return <a
            key={key ?? label}
            role='button'
            tabIndex={0}
            onClick={() => run()}
            onKeyDown={(event: React.KeyboardEvent) => {
                if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    run();
                }
            }}
        >
            {label}
        </a>;
    }
}
