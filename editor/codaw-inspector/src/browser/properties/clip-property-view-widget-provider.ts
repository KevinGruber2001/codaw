/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { DefaultPropertyViewWidgetProvider } from '@theia/property-view/lib/browser/property-view-widget-provider';
import { ClipPropertiesWidget } from './clip-properties-widget.js';
import { ClipSelection } from './clip-property-data-service.js';

/**
 * Tells the Inspector that a selected clip is shown with `ClipPropertiesWidget`. The view asks every
 * provider; the highest `canHandle` wins (the built-in one for files answers for files only).
 */
@injectable()
export class ClipPropertyViewWidgetProvider extends DefaultPropertyViewWidgetProvider {

    override id = 'codaw.clip';
    override label = 'Clip';

    @inject(ClipPropertiesWidget)
    protected readonly widget: ClipPropertiesWidget;

    override canHandle(selection: Object | undefined): number {
        return ClipSelection.is(selection) ? 100 : 0;
    }

    override async provideWidget(): Promise<ClipPropertiesWidget> {
        return this.widget;
    }

    override updateContentWidget(selection: Object | undefined): void {
        this.getPropertyDataService(selection).then(service => this.widget.updatePropertyViewContent(service, selection));
    }
}
