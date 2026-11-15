/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { PreferenceSchema } from '@theia/core/lib/common/preferences';

export const ENGINE_PATH_PREFERENCE = 'codaw.engine.path';

export const enginePreferenceSchema: PreferenceSchema = {
    properties: {
        [ENGINE_PATH_PREFERENCE]: {
            type: 'string',
            default: 'codaw',
            description: 'The CodaW audio engine: the path of the `codaw` binary, or just `codaw` to find it on the PATH.'
        }
    }
};
