/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution, Widget } from '@theia/core/lib/browser';
import { ApplicationShell } from '@theia/core/lib/browser/shell/application-shell';
import { AbstractViewContribution } from '@theia/core/lib/browser/shell/view-contribution';
import { ClipEditorViewContribution } from 'codaw-clip-editor/lib/browser/clip-editor-view-contribution';
import { EngineLogViewContribution } from 'codaw-engine/lib/browser/engine-log-view-contribution';
import { InspectorViewContribution } from 'codaw-inspector/lib/browser/inspector-view-contribution';
import { LibraryViewContribution } from 'codaw-library/lib/browser/library-view-contribution';
import { MixerViewContribution } from 'codaw-mixer/lib/browser/mixer-view-contribution';

/** The bottom panel's height on the first start, in pixels. */
const BOTTOM_PANEL_HEIGHT = 340;

/**
 * CodaW's default layout: which views are open on the first start, in what order, and which one of
 * each area is in front. The arrangement is not among them: it is always there (see the shell). Where each view goes is the feature's own business (its view contribution's
 * `defaultWidgetOptions`); which of them make up the first impression is the product's.
 *
 * Theia calls `initializeLayout` only when there is no saved layout to restore: on the first start, and
 * after "Reset Workbench Layout". From then on, wherever the user moved the views wins.
 */
@injectable()
export class CodawLayoutContribution implements FrontendApplicationContribution {

    @inject(LibraryViewContribution) protected readonly library: LibraryViewContribution;
    @inject(InspectorViewContribution) protected readonly inspector: InspectorViewContribution;
    @inject(MixerViewContribution) protected readonly mixer: MixerViewContribution;
    @inject(ClipEditorViewContribution) protected readonly clipEditor: ClipEditorViewContribution;
    @inject(EngineLogViewContribution) protected readonly engineLog: EngineLogViewContribution;

    @inject(ApplicationShell) protected readonly shell: ApplicationShell;

    async initializeLayout(): Promise<void> {
        // Opened in this order. The side panels sort their views by rank anyway, but the main area and
        // the bottom panel add a tab next to the active one, so each view is placed after the previous
        // one of the same area explicitly.
        const views: AbstractViewContribution<Widget>[] = [
            this.library, this.inspector, this.mixer, this.clipEditor, this.engineLog
        ];
        const previous = new Map<string, Widget>();
        for (const view of views) {
            const area = view.defaultViewOptions.area ?? 'main';
            const ref = previous.get(area);
            previous.set(area, await view.openView(ref ? { reveal: false, ref, mode: 'tab-after' } : { reveal: false }));
        }

        // The one in front in each area.
        await this.inspector.openView({ reveal: true });
        await this.mixer.openView({ reveal: true });

        // Tall enough for the mixer's faders; Theia's default bottom panel is sized for a terminal.
        this.shell.resize(BOTTOM_PANEL_HEIGHT, 'bottom');
    }
}
