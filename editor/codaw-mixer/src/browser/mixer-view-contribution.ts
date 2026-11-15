/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { injectable } from '@theia/core/shared/inversify';
import { AbstractViewContribution } from '@theia/core/lib/browser/shell/view-contribution';
import { MixerWidget } from './mixer-widget.js';

/**
 * Opens, closes and toggles the Mixer view, and says where it goes when opened: the bottom area.
 * The base class registers the "Toggle Mixer" command and the entry in View > Views.
 */
@injectable()
export class MixerViewContribution extends AbstractViewContribution<MixerWidget> {

    constructor() {
        super({
            widgetId: MixerWidget.ID,
            widgetName: MixerWidget.LABEL,
            defaultWidgetOptions: { area: 'bottom', rank: 100 },
            toggleCommandId: `${MixerWidget.ID}.toggle`
        });
    }
}
