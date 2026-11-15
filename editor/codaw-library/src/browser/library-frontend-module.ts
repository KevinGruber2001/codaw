/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/library.css';

import { ContainerModule } from '@theia/core/shared/inversify';
import { WidgetFactory, bindViewContribution } from '@theia/core/lib/browser';
import { LibraryViewContribution } from './library-view-contribution.js';
import { LibraryWidget } from './library-widget.js';
import { SamplePreview } from './sample-preview.js';

export default new ContainerModule(bind => {
    bind(SamplePreview).toSelf().inSingletonScope();

    // The widget, created by the WidgetManager through the factory with its id.
    bind(LibraryWidget).toSelf().inSingletonScope();
    bind(WidgetFactory).toDynamicValue(({ container }) => ({
        id: LibraryWidget.ID,
        createWidget: () => container.get(LibraryWidget)
    })).inSingletonScope();

    // Binds the contribution and registers it as a command, menu and keybinding contribution.
    bindViewContribution(bind, LibraryViewContribution);
});
