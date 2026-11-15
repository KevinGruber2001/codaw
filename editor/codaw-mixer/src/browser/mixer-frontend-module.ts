/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/mixer.css';

import { ContainerModule } from '@theia/core/shared/inversify';
import { WidgetFactory, bindViewContribution } from '@theia/core/lib/browser';
import { MixerViewContribution } from './mixer-view-contribution.js';
import { MixerWidget } from './mixer-widget.js';

export default new ContainerModule(bind => {
    // The widget, created by the WidgetManager through the factory with its id.
    bind(MixerWidget).toSelf();
    bind(WidgetFactory).toDynamicValue(({ container }) => ({
        id: MixerWidget.ID,
        createWidget: () => container.get(MixerWidget)
    })).inSingletonScope();

    // Binds the contribution and registers it as a command, menu and keybinding contribution.
    bindViewContribution(bind, MixerViewContribution);
});
