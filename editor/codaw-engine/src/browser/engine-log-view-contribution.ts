/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { injectable } from '@theia/core/shared/inversify';
import { AbstractViewContribution } from '@theia/core/lib/browser/shell/view-contribution';
import { EngineLogWidget } from './engine-log-widget.js';

/**
 * Opens, closes and toggles the Engine Log view, and says where it goes when opened: the bottom area.
 * The base class registers the "Toggle Engine Log" command and the entry in View > Views.
 */
@injectable()
export class EngineLogViewContribution extends AbstractViewContribution<EngineLogWidget> {

    constructor() {
        super({
            widgetId: EngineLogWidget.ID,
            widgetName: EngineLogWidget.LABEL,
            defaultWidgetOptions: { area: 'bottom', rank: 300 },
            toggleCommandId: `${EngineLogWidget.ID}.toggle`
        });
    }
}
