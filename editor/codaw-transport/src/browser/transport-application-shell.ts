/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { BoxLayout, BoxPanel, Layout } from '@theia/core/shared/@lumino/widgets';
import { inject, injectable } from '@theia/core/shared/inversify';

import { ApplicationShell } from '@theia/core/lib/browser/shell/application-shell';
import { TransportWidget } from './transport-widget.js';

/**
 * The shell, with the transport bar as a row of its own between the top panel and the work area.
 *
 * The shell's `top` area is not the place for it: that is the menu bar's row, laid out side by side
 * with the logo, and the desktop application hides it altogether whenever the menu is native, which
 * it is by default on macOS. A transport added there would disappear in exactly the build that matters
 * most. `createLayout` is the hook Theia provides for rearranging the shell, so the transport is added
 * to the layout the base class assembles rather than to one of its areas.
 */
@injectable()
export class TransportApplicationShell extends ApplicationShell {

    @inject(TransportWidget)
    protected readonly transportWidget: TransportWidget;

    protected createLayout(): Layout {
        // The base layout stacks the top panel, the work area and the status bar, top to bottom.
        const layout = super.createLayout() as BoxLayout;
        // A stretch of 0 keeps the row at its own height while the work area takes the rest.
        BoxPanel.setStretch(this.transportWidget, 0);
        layout.insertWidget(1, this.transportWidget);
        return layout;
    }
}
