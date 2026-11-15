/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { PreferenceSchema } from '@theia/core/lib/common/preferences';

export const SHOW_WELCOME_ON_STARTUP = 'codaw.welcome.showOnStartup';

/**
 * The welcome page is CodaW's own, so the preference that governs it is too. Theia's
 * `workbench.startupEditor` is contributed by `@theia/getting-started`, which this application
 * deliberately does not depend on.
 */
export const codawWelcomePreferenceSchema: PreferenceSchema = {
    title: 'CodaW',
    properties: {
        [SHOW_WELCOME_ON_STARTUP]: {
            type: 'boolean',
            default: true,
            description: 'Show the Welcome page when CodaW starts with nothing else open.'
        }
    }
};
