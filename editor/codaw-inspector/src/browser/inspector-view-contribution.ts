/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution, WidgetManager, codicon } from '@theia/core/lib/browser';
import { PropertyViewContribution } from '@theia/property-view/lib/browser/property-view-contribution';
import { PropertyViewWidget } from '@theia/property-view/lib/browser/property-view-widget';

export const INSPECTOR_LABEL = 'Inspector';
const INSPECTOR_ICON = 'symbol-property';

/**
 * The Inspector is Theia's Properties view (`@theia/property-view`), under CodaW's name and in the right
 * side panel. The property view follows the selection and shows whatever a `PropertyViewWidgetProvider`
 * contributes for it; CodaW's providers are in `properties/`.
 *
 * Replaces `PropertyViewContribution` (see the module), so the View menu, the toggle command and the
 * default place all say Inspector.
 */
@injectable()
export class InspectorViewContribution extends PropertyViewContribution implements FrontendApplicationContribution {

    @inject(WidgetManager)
    protected readonly widgetManager: WidgetManager;

    constructor() {
        super();
        // The base class's options are its own literal: changed here, before anything reads them.
        this.options.widgetName = INSPECTOR_LABEL;
        this.options.defaultWidgetOptions = { area: 'right', rank: 100 };
    }

    onStart(): void {
        // The widget names itself "Properties" when created; it is the Inspector here.
        this.widgetManager.onDidCreateWidget(({ widget }) => {
            if (widget.id === PropertyViewWidget.ID) {
                widget.title.label = INSPECTOR_LABEL;
                widget.title.caption = INSPECTOR_LABEL;
                widget.title.iconClass = codicon(INSPECTOR_ICON);
            }
        });
    }
}
