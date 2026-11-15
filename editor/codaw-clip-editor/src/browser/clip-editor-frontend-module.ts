/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { ContainerModule } from '@theia/core/shared/inversify';
import { WidgetFactory, bindViewContribution } from '@theia/core/lib/browser';
import { ClipEditorViewContribution } from './clip-editor-view-contribution.js';
import { ClipEditorWidget } from './clip-editor-widget.js';

export default new ContainerModule(bind => {
    // The widget, created by the WidgetManager through the factory with its id.
    bind(ClipEditorWidget).toSelf();
    bind(WidgetFactory).toDynamicValue(({ container }) => ({
        id: ClipEditorWidget.ID,
        createWidget: () => container.get(ClipEditorWidget)
    })).inSingletonScope();

    // Binds the contribution and registers it as a command, menu and keybinding contribution.
    bindViewContribution(bind, ClipEditorViewContribution);
});
