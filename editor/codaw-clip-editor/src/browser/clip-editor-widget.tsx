/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, codicon } from '@theia/core/lib/browser/widgets';

/**
 * The Clip Editor view. A placeholder for now: it only says what it will become.
 */
@injectable()
export class ClipEditorWidget extends ReactWidget {

    static readonly ID = 'codaw.clipEditor';
    static readonly LABEL = 'Clip Editor';
    static readonly ICON = 'edit';

    @postConstruct()
    protected init(): void {
        this.id = ClipEditorWidget.ID;
        this.title.label = ClipEditorWidget.LABEL;
        this.title.caption = ClipEditorWidget.LABEL;
        this.title.iconClass = codicon(ClipEditorWidget.ICON);
        this.title.closable = true;
        this.addClass('codaw-placeholder');
        this.update();
    }

    protected render(): React.ReactNode {
        return <div className='codaw-placeholder-content'>
            <i className={`${codicon(ClipEditorWidget.ICON)} codaw-placeholder-icon`} />
            <h2 className='codaw-placeholder-title'>{ClipEditorWidget.LABEL}</h2>
            <p className='codaw-placeholder-description'>
                The selected clip up close: a piano roll for notes, a waveform for audio.
            </p>
        </div>;
    }
}
