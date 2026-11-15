/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { injectable } from '@theia/core/shared/inversify';
import { AbstractViewContribution } from '@theia/core/lib/browser/shell/view-contribution';
import { ClipEditorWidget } from './clip-editor-widget.js';

/**
 * Opens, closes and toggles the Clip Editor view, and says where it goes when opened: the bottom area.
 * The base class registers the "Toggle Clip Editor" command and the entry in View > Views.
 */
@injectable()
export class ClipEditorViewContribution extends AbstractViewContribution<ClipEditorWidget> {

    constructor() {
        super({
            widgetId: ClipEditorWidget.ID,
            widgetName: ClipEditorWidget.LABEL,
            defaultWidgetOptions: { area: 'bottom', rank: 200 },
            toggleCommandId: `${ClipEditorWidget.ID}.toggle`
        });
    }
}
