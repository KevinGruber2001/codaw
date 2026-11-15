/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * Adapted from the Eclipse Theia IDE (theia-extensions/product), which is
 * available under the terms of the MIT License.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';

import { ABOUT_CONTENT_CLASS, AboutDialog, AboutDialogProps } from '@theia/core/lib/browser/about-dialog';
import { inject, injectable } from '@theia/core/shared/inversify';
import { renderBrandingSections, renderProductName } from './branding-util.js';

import { WindowService } from '@theia/core/lib/browser/window/window-service';

@injectable()
export class CodawAboutDialog extends AboutDialog {

    @inject(WindowService)
    protected readonly windowService: WindowService;

    constructor(
        @inject(AboutDialogProps) protected readonly props: AboutDialogProps
    ) {
        super(props);
    }

    protected render(): React.ReactNode {
        return <div className={ABOUT_CONTENT_CLASS}>
            {this.renderContent()}
        </div>;
    }

    protected renderContent(): React.ReactNode {
        return <div className='ad-container'>
            <div className='ad-float'>
                <div className='ad-logo'>
                </div>
                {this.renderExtensions()}
            </div>
            {this.renderTitle()}
            <hr className='codaw-rule' />
            {renderBrandingSections({ windowService: this.windowService })}
        </div>;
    }

    protected renderTitle(): React.ReactNode {
        return <div className='codaw-about-header'>
            {renderProductName()}
            {this.renderVersion()}
        </div>;
    }

    protected renderVersion(): React.ReactNode {
        return <p className='codaw-subtitle'>
            {this.applicationInfo ? 'Version ' + this.applicationInfo.version : '-'}
        </p>;
    }
}
