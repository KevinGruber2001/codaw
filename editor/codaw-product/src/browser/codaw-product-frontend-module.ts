/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * Adapted from the Eclipse Theia IDE (theia-extensions/product), which is
 * available under the terms of the MIT License.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/index.css';
import '../../src/browser/style/placeholder.css';
import '../../src/browser/style/file-slot.css';

import { CommandContribution, MenuContribution } from '@theia/core/lib/common';
import { PreferenceContribution } from '@theia/core/lib/common/preferences';

import { AboutDialog } from '@theia/core/lib/browser/about-dialog';
import { CodawAboutDialog } from './codaw-about-dialog.js';
import { CodawContribution } from './codaw-contribution.js';
import { CodawLayoutContribution } from './codaw-layout-contribution.js';
import { CodawTrimContribution } from './codaw-trim-contribution.js';
import { CodawApplicationShell } from './codaw-application-shell.js';
import { FileSlotHeader } from './file-slot-header.js';
import { ApplicationShell } from '@theia/core/lib/browser/shell/application-shell';
import { FilterContribution } from '@theia/core/lib/common/contribution-filter';
import { CodawWelcomeContribution } from './codaw-welcome-contribution.js';
import { CodawWelcomeWidget } from './codaw-welcome-widget.js';
import { ContainerModule } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution, WidgetFactory } from '@theia/core/lib/browser';
import { codawWelcomePreferenceSchema } from './codaw-welcome-preferences.js';

export default new ContainerModule((bind, _unbind, isBound, rebind) => {
    // The shell: the arrangement as the fixed main view, Theia's main area beside it as a one-file slot.
    // Replaces the transport's shell, which it extends (the shell can only be replaced once).
    bind(FileSlotHeader).toSelf().inSingletonScope();
    rebind(ApplicationShell).to(CodawApplicationShell).inSingletonScope();

    // The default layout, assembled from the feature packages' views.
    bind(CodawLayoutContribution).toSelf().inSingletonScope();
    bind(FrontendApplicationContribution).toService(CodawLayoutContribution);

    // What is left out of Theia: views and menus a DAW does not need.
    bind(CodawTrimContribution).toSelf().inSingletonScope();
    bind(FilterContribution).toService(CodawTrimContribution);
    bind(MenuContribution).toService(CodawTrimContribution);

    // The welcome page.
    bind(CodawWelcomeWidget).toSelf();
    bind(WidgetFactory).toDynamicValue(context => ({
        id: CodawWelcomeWidget.ID,
        createWidget: () => context.container.get<CodawWelcomeWidget>(CodawWelcomeWidget)
    })).inSingletonScope();
    bind(PreferenceContribution).toConstantValue({ schema: codawWelcomePreferenceSchema });
    bind(CodawWelcomeContribution).toSelf().inSingletonScope();
    [CommandContribution, MenuContribution, FrontendApplicationContribution].forEach(serviceIdentifier =>
        bind(serviceIdentifier).toService(CodawWelcomeContribution)
    );

    if (isBound(AboutDialog)) {
        rebind(AboutDialog).to(CodawAboutDialog).inSingletonScope();
    } else {
        bind(AboutDialog).to(CodawAboutDialog).inSingletonScope();
    }

    bind(CodawContribution).toSelf().inSingletonScope();
    [CommandContribution, MenuContribution].forEach(serviceIdentifier =>
        bind(serviceIdentifier).toService(CodawContribution)
    );
});
