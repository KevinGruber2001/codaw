/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { ContainerModule } from '@theia/core/shared/inversify';
import { WidgetFactory, bindViewContribution } from '@theia/core/lib/browser';
import { LibraryViewContribution } from './library-view-contribution.js';
import { LibraryWidget } from './library-widget.js';

export default new ContainerModule(bind => {
    // The widget, created by the WidgetManager through the factory with its id.
    bind(LibraryWidget).toSelf();
    bind(WidgetFactory).toDynamicValue(({ container }) => ({
        id: LibraryWidget.ID,
        createWidget: () => container.get(LibraryWidget)
    })).inSingletonScope();

    // Binds the contribution and registers it as a command, menu and keybinding contribution.
    bindViewContribution(bind, LibraryViewContribution);
});
