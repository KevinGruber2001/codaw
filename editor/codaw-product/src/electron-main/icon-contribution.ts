/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * Adapted from the Eclipse Theia IDE (theia-extensions/product), which is
 * available under the terms of the MIT License.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as os from 'os';
import * as path from 'path';

import { ElectronMainApplication, ElectronMainApplicationContribution } from '@theia/core/lib/electron-main/electron-main-application';

import { BrowserWindow } from '@theia/core/electron-shared/electron';
import { injectable } from '@theia/core/shared/inversify';

@injectable()
export class IconContribution implements ElectronMainApplicationContribution {

    onStart(application: ElectronMainApplication): void {
        if (os.platform() !== 'linux') {
            return;
        }
        const windowOptions = application.config.electron.windowOptions;
        if (!windowOptions || windowOptions.icon !== undefined) {
            return;
        }
        // The window image is undefined. If the executable has an image set, that is used as a fallback,
        // but AppImage no longer supports this via electron-builder, so Linux gets an explicit one.
        // The path is relative to the bundled backend, which the packaged application puts in `lib/backend`.
        const icon = path.join(import.meta.dirname, '../../resources/icons/WindowIcon/512-512.png');
        windowOptions.icon = icon;
        // Also update any window that already exists, e.g. the splash screen.
        for (const window of BrowserWindow.getAllWindows()) {
            window.setIcon(icon);
        }
    }
}
