/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, Widget, codicon } from '@theia/core/lib/browser/widgets';
import { Saveable } from '@theia/core/lib/browser/saveable';
import { TheiaDockPanel } from '@theia/core/lib/browser/shell/theia-dock-panel';

/**
 * The header of the file slot, in place of tabs: the file shown, whether it has unsaved changes, and a
 * button to close it. Files with unsaved changes that were replaced by another one are still open behind
 * it; they are listed here, to switch back to.
 */
@injectable()
export class FileSlotHeader extends ReactWidget {

    static readonly ID = 'codaw-file-slot-header';

    protected panel?: TheiaDockPanel;

    @postConstruct()
    protected init(): void {
        this.id = FileSlotHeader.ID;
        this.addClass('codaw-file-slot-header');
    }

    /** Follows the dock panel whose files this is the header of. */
    connect(panel: TheiaDockPanel): void {
        this.panel = panel;
        const onTitleChanged = () => this.update();
        panel.widgetAdded.connect((_, widget) => {
            widget.title.changed.connect(onTitleChanged);
            this.update();
        });
        panel.widgetRemoved.connect((_, widget) => {
            widget.title.changed.disconnect(onTitleChanged);
            this.update();
        });
        panel.widgetActivated.connect(() => this.update());
        panel.layoutModified.connect(() => this.update());
        this.update();
    }

    protected get current(): Widget | undefined {
        return this.panel?.selectedWidgets().next().value ?? undefined;
    }

    protected render(): React.ReactNode {
        const panel = this.panel;
        const current = this.current;
        if (!panel || !current) {
            return undefined;
        }
        const others = Array.from(panel.widgets()).filter(widget => widget !== current);
        return <React.Fragment>
            <span className={`codaw-file-slot-icon ${current.title.iconClass}`} />
            <span className='codaw-file-slot-label' title={current.title.caption}>{current.title.label}</span>
            {Saveable.isDirty(current) && <span className='codaw-file-slot-dirty' title='Unsaved changes'>●</span>}
            <span className='codaw-file-slot-path'>{current.title.caption}</span>
            <span className='codaw-file-slot-spacer' />
            {others.map(widget =>
                <button key={widget.id} className='codaw-file-slot-other' title={`Unsaved: ${widget.title.caption}`}
                    onClick={() => panel.activateWidget(widget)}>
                    {widget.title.label}{Saveable.isDirty(widget) && ' ●'}
                </button>
            )}
            <button className={`codaw-file-slot-close ${codicon('close')}`} title='Close'
                onClick={() => current.close()} />
        </React.Fragment>;
    }
}
