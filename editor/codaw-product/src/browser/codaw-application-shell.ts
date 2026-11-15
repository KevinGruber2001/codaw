/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { BoxPanel, Layout, SplitPanel, Widget } from '@theia/core/shared/@lumino/widgets';
import { ApplicationShell } from '@theia/core/lib/browser/shell/application-shell';
import { TheiaSplitPanel } from '@theia/core/lib/browser/shell/theia-split-panel';
import { Saveable } from '@theia/core/lib/browser/saveable';
import { TransportApplicationShell } from 'codaw-transport/lib/browser/transport-application-shell';
import { ArrangementWidget } from 'codaw-arrangement/lib/browser/arrangement-widget';
import { FileSlotHeader } from './file-slot-header.js';

/** How much of the row the file slot takes when it opens. */
const FILE_SLOT_SHARE = 0.42;

/**
 * CodaW's shell. On top of the transport bar (from the transport's shell, which this extends: the shell
 * can be replaced only once, so the replacements have to build on each other), it changes the middle:
 *
 *     ┌ transport ─────────────────────────────────────────────┐
 *     │ left │ ARRANGEMENT              ║ file slot  │ right    │
 *     │      │ (fixed, the main view)   ║ (one file) │          │
 *     │      ├──────────────────────────╨────────────┤          │
 *     │      │ bottom panel                          │          │
 *
 * The arrangement is not a tab: it is always there and cannot be closed or moved. Theia's main area
 * becomes the file slot beside it: in single-document mode, so it shows one file, without tabs (a header
 * instead), and it collapses when no file is open. Clicking something in the arrangement opens its file
 * there. Because the slot is Theia's own main area, editing works as anywhere in Theia: save, find, undo,
 * "save before closing?".
 */
@injectable()
export class CodawApplicationShell extends TransportApplicationShell {

    @inject(ArrangementWidget)
    protected readonly arrangement: ArrangementWidget;

    @inject(FileSlotHeader)
    protected readonly fileSlotHeader: FileSlotHeader;

    protected mainRow: SplitPanel;
    protected fileSlot: BoxPanel;

    protected override createLayout(): Layout {
        const layout = super.createLayout();

        // The main area sits in a vertical split with the bottom panel. It is taken out of there, put into
        // the file slot under its header, and the row [arrangement | file slot] takes its place.
        const bottomSplit = this.mainPanel.parent as SplitPanel;
        const index = bottomSplit.widgets.indexOf(this.mainPanel);

        this.mainPanel.mode = 'single-document';
        this.fileSlotHeader.connect(this.mainPanel);
        this.fileSlot = new BoxPanel({ direction: 'top-to-bottom', spacing: 0 });
        this.fileSlot.id = 'codaw-file-slot';
        this.fileSlot.addWidget(this.fileSlotHeader);
        this.fileSlot.addWidget(this.mainPanel);
        BoxPanel.setStretch(this.fileSlotHeader, 0);
        BoxPanel.setStretch(this.mainPanel, 1);

        this.mainRow = new TheiaSplitPanel({
            layout: this.createSplitLayout([this.arrangement, this.fileSlot], [1, 0], { orientation: 'horizontal', spacing: 0 })
        });
        this.mainRow.id = 'codaw-main-row';
        bottomSplit.insertWidget(index, this.mainRow);

        this.mainPanel.widgetAdded.connect((_, widget) => this.onFileOpened(widget));
        // Switching back to a file kept open (from the header) makes the one before go, too, if it is clean.
        this.mainPanel.widgetActivated.connect((_, widget) => queueMicrotask(() => this.closeCleanFilesBut(widget)));
        this.mainPanel.widgetRemoved.connect(() => this.updateFileSlot());
        this.updateFileSlot();
        return layout;
    }

    /**
     * Restoring a saved layout puts the main area back into multiple-document mode (Lumino resets the mode
     * on restore); it is one file at a time here, so it goes back to single-document.
     */
    override async setLayoutData(layoutData: ApplicationShell.LayoutData): Promise<void> {
        await super.setLayoutData(layoutData);
        this.mainPanel.mode = 'single-document';
        this.closeCleanFilesBut(this.mainPanel.selectedWidgets().next().value ?? undefined);
        this.updateFileSlot();
    }

    /** A file was opened in the slot: it replaces the one before, unless that one has unsaved changes. */
    protected onFileOpened(widget: Widget): void {
        this.updateFileSlot();
        // After the add is done: closing in the middle of it would change the panel while it is adding.
        queueMicrotask(() => this.closeCleanFilesBut(widget));
    }

    /**
     * Closes every file in the slot except `keep`, as long as it has no unsaved changes. Those stay open
     * behind the one shown (the header lists them), so switching files never asks "save?".
     */
    protected closeCleanFilesBut(keep: Widget | undefined): void {
        for (const widget of Array.from(this.mainPanel.widgets())) {
            if (widget !== keep && !Saveable.isDirty(widget)) {
                widget.close();
            }
        }
    }

    /** The slot is there only while it has a file in it; the arrangement takes the whole row otherwise. */
    protected updateFileSlot(): void {
        if (!this.fileSlot) {
            return;
        }
        const empty = this.mainPanel.isEmpty;
        if (empty && !this.fileSlot.isHidden) {
            this.fileSlot.hide();
        } else if (!empty && this.fileSlot.isHidden) {
            this.fileSlot.show();
            this.mainRow.setRelativeSizes([1 - FILE_SLOT_SHARE, FILE_SLOT_SHARE]);
        }
    }
}
