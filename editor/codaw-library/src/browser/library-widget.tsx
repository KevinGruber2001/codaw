/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, codicon } from '@theia/core/lib/browser/widgets';

/**
 * The Library view. A placeholder for now: it only says what it will become.
 */
@injectable()
export class LibraryWidget extends ReactWidget {

    static readonly ID = 'codaw.library';
    static readonly LABEL = 'Library';
    static readonly ICON = 'library';

    @postConstruct()
    protected init(): void {
        this.id = LibraryWidget.ID;
        this.title.label = LibraryWidget.LABEL;
        this.title.caption = LibraryWidget.LABEL;
        this.title.iconClass = codicon(LibraryWidget.ICON);
        this.title.closable = true;
        this.addClass('codaw-placeholder');
        this.update();
    }

    protected render(): React.ReactNode {
        return <div className='codaw-placeholder-content'>
            <i className={`${codicon(LibraryWidget.ICON)} codaw-placeholder-icon`} />
            <h2 className='codaw-placeholder-title'>{LibraryWidget.LABEL}</h2>
            <p className='codaw-placeholder-description'>
                Samples, instruments and effects to drag into the project. Next to the Explorer, which shows the project's files.
            </p>
        </div>;
    }
}
