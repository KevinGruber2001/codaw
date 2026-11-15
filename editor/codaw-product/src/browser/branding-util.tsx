/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * Adapted from the Eclipse Theia IDE (theia-extensions/product), which is
 * available under the terms of the MIT License.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';

import { WindowService } from '@theia/core/lib/browser/window/window-service';
import { codicon } from '@theia/core/lib/browser';

export const REPOSITORY_URL = 'https://github.com/KevinGruber2001/codaw';
export const ISSUES_URL = `${REPOSITORY_URL}/issues/new`;
export const RELEASES_URL = `${REPOSITORY_URL}/releases`;
export const DOCUMENTATION_URL = `${REPOSITORY_URL}#readme`;

export interface BrandingProps {
    windowService: WindowService;
}

/**
 * The product name as it is set in type: the accent on the last letter mirrors the accent bar in the
 * application icon, so the wordmark and the icon read as the same mark.
 */
export function renderProductName(): React.ReactNode {
    return <h1>Coda<span className='codaw-branding-accent'>W</span></h1>;
}

export interface ExternalLinkProps extends BrandingProps {
    text: string;
    url: string;
}

/**
 * Opens in the system browser rather than in a window of the application, which is what the user expects
 * of an external link in the desktop build.
 */
function BrowserLink(props: ExternalLinkProps): React.JSX.Element {
    const open = () => props.windowService.openNewWindow(props.url, { external: true });
    return <a
        role={'button'}
        tabIndex={0}
        onClick={open}
        onKeyDown={e => {
            if (e.key === 'Enter' || e.key === ' ') {
                open();
            }
        }}
    >
        {props.text}
    </a>;
}

/*
 * The sections below are shared by the welcome page and the About dialog, so that both describe the
 * product in the same words. Keep each to a sentence or two: the welcome page has to stay scannable.
 */

export function renderWhatIs(windowService: WindowService): React.ReactNode {
    return <div className='codaw-section'>
        <h3 className='codaw-section-header'>
            <i className={codicon('info')}></i>
            What is CodaW?
        </h3>
        <div>
            CodaW is a digital audio workstation whose session is code: tracks, buses, clips, effects and
            automation all live in plain-text TOML that you can version, diff and hand-edit.
        </div>
        <div>
            Instead of hunting through mixer pages, you edit the file for the thing you want to change and
            save it. The running engine picks the change up live.
        </div>
    </div>;
}

export function renderEngine(windowService: WindowService): React.ReactNode {
    return <div className='codaw-section'>
        <h3 className='codaw-section-header'>
            <i className={codicon('pulse')}></i>
            The audio engine
        </h3>
        <div>
            Playback, hot reload and offline rendering are done by the <span className='codaw-text-bold'>codaw</span> engine,
            a Go command line program that plays a project, re-reads it as you edit, and bounces it to a WAV
            file. Prebuilt binaries are on
            the <BrowserLink text='releases page' url={RELEASES_URL} windowService={windowService} />.
        </div>
    </div>;
}

export function renderProjectStatus(windowService: WindowService): React.ReactNode {
    return <div className='codaw-section'>
        <h3 className='codaw-section-header'>
            <i className={codicon('beaker')}></i>
            This desktop application is early
        </h3>
        <div>
            For now it is the editor shell: the arrangement, mixer and transport views are still being built,
            so a project is edited as the TOML files it is made of, with the engine run alongside it.
        </div>
    </div>;
}

export function renderCommunity(windowService: WindowService): React.ReactNode {
    return <div className='codaw-section'>
        <h3 className='codaw-section-header'>
            <i className={codicon('comment-discussion')}></i>
            Source &amp; issues
        </h3>
        <div>
            The engine, the editor and the project format are developed in
            one <BrowserLink text='repository' url={REPOSITORY_URL} windowService={windowService} /> on GitHub.
            Bugs and feature requests
            go <BrowserLink text='there' url={ISSUES_URL} windowService={windowService} /> as well.
        </div>
    </div>;
}

/**
 * The product description as shown by both the welcome page and the About dialog. Sharing the whole list
 * rather than the individual sections keeps the two from drifting apart as sections come and go.
 */
export function renderBrandingSections(props: BrandingProps): React.ReactNode {
    return <React.Fragment>
        {renderWhatIs(props.windowService)}
        {renderEngine(props.windowService)}
        {renderProjectStatus(props.windowService)}
        {renderCommunity(props.windowService)}
    </React.Fragment>;
}
