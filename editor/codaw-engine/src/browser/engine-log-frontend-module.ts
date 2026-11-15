/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/engine.css';

import { CommandContribution } from '@theia/core/lib/common/command';
import { ContainerModule } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution, WidgetFactory, bindViewContribution } from '@theia/core/lib/browser';
import { EngineLogViewContribution } from './engine-log-view-contribution.js';
import { EngineLogWidget } from './engine-log-widget.js';
import { EngineFrontendContribution } from './engine-frontend-contribution.js';
import { EngineService } from './engine-service.js';
import { EngineStatusBarContribution } from './engine-status-bar-contribution.js';
import { PreferenceContribution } from '@theia/core/lib/common/preferences';
import { enginePreferenceSchema } from './engine-preferences.js';

export default new ContainerModule(bind => {
    // The widget, created by the WidgetManager through the factory with its id.
    bind(EngineLogWidget).toSelf();
    bind(WidgetFactory).toDynamicValue(({ container }) => ({
        id: EngineLogWidget.ID,
        createWidget: () => container.get(EngineLogWidget)
    })).inSingletonScope();

    // Binds the contribution and registers it as a command, menu and keybinding contribution.
    bindViewContribution(bind, EngineLogViewContribution);

    // The engine: the service the rest of the frontend talks to, and the setting that says where it is.
    bind(EngineService).toSelf().inSingletonScope();
    bind(PreferenceContribution).toConstantValue({ schema: enginePreferenceSchema });

    // Starts the engine when a project is open.
    bind(EngineFrontendContribution).toSelf().inSingletonScope();
    bind(FrontendApplicationContribution).toService(EngineFrontendContribution);
    bind(CommandContribution).toService(EngineFrontendContribution);

    // Its status, in the status bar.
    bind(EngineStatusBarContribution).toSelf().inSingletonScope();
    bind(FrontendApplicationContribution).toService(EngineStatusBarContribution);
});
