/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/arrangement.css';

import { ContainerModule } from '@theia/core/shared/inversify';
import { CommandContribution } from '@theia/core/lib/common/command';
import { KeybindingContribution } from '@theia/core/lib/browser/keybinding';
import { ArrangementContribution } from './arrangement-contribution.js';
import { ArrangementWidget } from './arrangement-widget.js';

export default new ContainerModule(bind => {
    // One arrangement, for the application shell to place as the main view (see codaw-product's shell).
    bind(ArrangementWidget).toSelf().inSingletonScope();

    bind(ArrangementContribution).toSelf().inSingletonScope();
    bind(CommandContribution).toService(ArrangementContribution);
    bind(KeybindingContribution).toService(ArrangementContribution);
});
