/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, codicon } from '@theia/core/lib/browser/widgets';
import { EngineService } from './engine-service.js';

/**
 * The Engine Log view: what the audio engine writes to stderr (loading, reloads, warnings), and what
 * happened to it (started, stopped, crashed). Opened late, it still shows the lines from before,
 * because `EngineService` keeps them.
 */
@injectable()
export class EngineLogWidget extends ReactWidget {

    static readonly ID = 'codaw.engineLog';
    static readonly LABEL = 'Engine Log';
    static readonly ICON = 'output';

    @inject(EngineService)
    protected readonly engine: EngineService;

    /** Whether the view sits at the bottom, and so should follow new lines. */
    protected following = true;

    @postConstruct()
    protected init(): void {
        this.id = EngineLogWidget.ID;
        this.title.label = EngineLogWidget.LABEL;
        this.title.caption = EngineLogWidget.LABEL;
        this.title.iconClass = codicon(EngineLogWidget.ICON);
        this.title.closable = true;
        this.addClass('codaw-engine-log');
        this.node.addEventListener('scroll', () => {
            this.following = this.node.scrollTop + this.node.clientHeight >= this.node.scrollHeight - 4;
        });
        this.toDispose.push(this.engine.onDidLog(() => this.update()));
        this.update();
    }

    protected onUpdateRequest(msg: Parameters<ReactWidget['onUpdateRequest']>[0]): void {
        super.onUpdateRequest(msg);
        if (this.following) {
            // After React has put the new line in.
            requestAnimationFrame(() => this.node.scrollTop = this.node.scrollHeight);
        }
    }

    protected render(): React.ReactNode {
        const lines = this.engine.log;
        if (lines.length === 0) {
            return <div className='codaw-engine-log-empty'>Nothing from the engine yet.</div>;
        }
        return <div className='codaw-engine-log-lines'>
            {lines.map((line, i) => <div key={i} className={lineClass(line)}>{line}</div>)}
        </div>;
    }
}

function lineClass(line: string): string {
    if (/\berror\b|panic|failed/i.test(line)) {
        return 'codaw-engine-log-line codaw-engine-log-error';
    }
    if (/\bwarn(ing)?\b/i.test(line)) {
        return 'codaw-engine-log-line codaw-engine-log-warning';
    }
    if (line.includes('[engine]')) {
        return 'codaw-engine-log-line codaw-engine-log-status';
    }
    return 'codaw-engine-log-line';
}
