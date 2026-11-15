/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { Command } from '@theia/core/lib/common/command';

export namespace TransportCommands {
    const CATEGORY = 'Transport';

    export const TOGGLE_PLAY: Command = { id: 'codaw.transport.togglePlay', category: CATEGORY, label: 'Play / Pause' };
    export const STOP: Command = { id: 'codaw.transport.stop', category: CATEGORY, label: 'Stop' };
    export const RETURN_TO_START: Command = { id: 'codaw.transport.returnToStart', category: CATEGORY, label: 'Return to Start' };
    export const REWIND: Command = { id: 'codaw.transport.rewind', category: CATEGORY, label: 'Rewind One Bar' };
    export const FORWARD: Command = { id: 'codaw.transport.forward', category: CATEGORY, label: 'Forward One Bar' };
    export const TOGGLE_LOOP: Command = { id: 'codaw.transport.toggleLoop', category: CATEGORY, label: 'Toggle Loop' };
}
