/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { injectable } from '@theia/core/shared/inversify';
import { AbstractViewContribution } from '@theia/core/lib/browser/shell/view-contribution';
import { LibraryWidget } from './library-widget.js';

/**
 * Opens, closes and toggles the Library view, and says where it goes when opened: the left area.
 * The base class registers the "Toggle Library" command and the entry in View > Views.
 */
@injectable()
export class LibraryViewContribution extends AbstractViewContribution<LibraryWidget> {

    constructor() {
        super({
            widgetId: LibraryWidget.ID,
            widgetName: LibraryWidget.LABEL,
            defaultWidgetOptions: { area: 'left', rank: 200 },
            toggleCommandId: `${LibraryWidget.ID}.toggle`
        });
    }
}
