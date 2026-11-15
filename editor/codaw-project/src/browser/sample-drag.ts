/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { injectable } from '@theia/core/shared/inversify';

/** The drag data type of a sample dragged from the Library; its value is the sample's JSON. */
export const SAMPLE_DRAG_MIME = 'application/x-codaw-sample';

/** A sample being dragged: the audio file, and how long it is if that is known yet. */
export interface DraggedSample {
    /** Relative to the project, as a clip's `file` says it. */
    file: string;
    uri: string;
    /** In seconds. */
    duration?: number;
}

/**
 * The sample being dragged right now, if any. A drop target can read the drag data only on drop, not
 * while the drag passes over it; this is what it shows a preview from in the meantime.
 */
@injectable()
export class SampleDragService {
    current?: DraggedSample;
}
