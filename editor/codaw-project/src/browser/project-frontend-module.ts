/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import '../../src/browser/style/tracks.css';

import { ContainerModule } from '@theia/core/shared/inversify';
import { AudioPeaksService } from './audio-peaks-service.js';
import { ProjectEditService } from './project-edit-service.js';
import { ProjectService } from './project-service.js';
import { ProjectNavigationContribution } from './project-navigation.js';
import { SampleDragService } from './sample-drag.js';
import { SchemaService } from './schema-service.js';
import { CommandContribution } from '@theia/core/lib/common/command';

export default new ContainerModule(bind => {
    bind(SchemaService).toSelf().inSingletonScope();
    bind(ProjectService).toSelf().inSingletonScope();
    bind(ProjectEditService).toSelf().inSingletonScope();
    bind(AudioPeaksService).toSelf().inSingletonScope();
    bind(SampleDragService).toSelf().inSingletonScope();
    bind(ProjectNavigationContribution).toSelf().inSingletonScope();
    bind(CommandContribution).toService(ProjectNavigationContribution);
});
