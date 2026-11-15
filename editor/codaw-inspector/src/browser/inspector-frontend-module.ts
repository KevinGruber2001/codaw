/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/properties.css';

import { ContainerModule } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution } from '@theia/core/lib/browser/frontend-application-contribution';
import { PropertyDataService } from '@theia/property-view/lib/browser/property-data-service';
import { PropertyViewContribution } from '@theia/property-view/lib/browser/property-view-contribution';
import { PropertyViewWidgetProvider } from '@theia/property-view/lib/browser/property-view-widget-provider';
import { InspectorViewContribution } from './inspector-view-contribution.js';
import { ClipPropertyDataService } from './properties/clip-property-data-service.js';
import { ClipPropertiesWidget } from './properties/clip-properties-widget.js';
import { ClipPropertyViewWidgetProvider } from './properties/clip-property-view-widget-provider.js';

export default new ContainerModule((bind, _unbind, _isBound, rebind) => {
    // The Inspector: Theia's property view, renamed and placed on the right. Everything the property view
    // module registered for its contribution (commands, menus, keys) now resolves to this one.
    bind(InspectorViewContribution).toSelf().inSingletonScope();
    rebind(PropertyViewContribution).toService(InspectorViewContribution);
    bind(FrontendApplicationContribution).toService(InspectorViewContribution);

    // A selected clip: the data service says what its properties are, the provider which widget shows them.
    bind(ClipPropertyDataService).toSelf().inSingletonScope();
    bind(PropertyDataService).toService(ClipPropertyDataService);
    bind(ClipPropertiesWidget).toSelf().inSingletonScope();
    bind(ClipPropertyViewWidgetProvider).toSelf().inSingletonScope();
    bind(PropertyViewWidgetProvider).toService(ClipPropertyViewWidgetProvider);
});
