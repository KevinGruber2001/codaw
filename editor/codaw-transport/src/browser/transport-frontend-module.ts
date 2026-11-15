/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/transport.css';

import { CommandContribution } from '@theia/core/lib/common';
import { ContainerModule } from '@theia/core/shared/inversify';
import { KeybindingContribution } from '@theia/core/lib/browser';
import { ApplicationShell } from '@theia/core/lib/browser/shell/application-shell';
import { TransportApplicationShell } from './transport-application-shell.js';
import { TransportContribution } from './transport-contribution.js';
import { TransportService } from './transport-service.js';
import { TransportWidget } from './transport-widget.js';

export default new ContainerModule((bind, _unbind, _isBound, rebind) => {
    // One service owning the state, the bar that shows it, and the commands and keys that drive it.
    bind(TransportService).toSelf().inSingletonScope();
    bind(TransportWidget).toSelf().inSingletonScope();
    bind(TransportContribution).toSelf().inSingletonScope();
    [CommandContribution, KeybindingContribution].forEach(serviceIdentifier =>
        bind(serviceIdentifier).toService(TransportContribution)
    );

    // The bar is not in one of the shell's areas but a row of its own, which takes replacing the shell.
    rebind(ApplicationShell).to(TransportApplicationShell).inSingletonScope();
});
