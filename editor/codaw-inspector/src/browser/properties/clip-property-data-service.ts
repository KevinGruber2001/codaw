/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { PropertyDataService } from '@theia/property-view/lib/browser/property-data-service';
import { ProjectService } from 'codaw-project/lib/browser/project-service';
import { AudioPeaksService } from 'codaw-project/lib/browser/audio-peaks-service';
import { ProjectClip } from 'codaw-project/lib/common/project-model';

/** What the arrangement puts in the SelectionService when a clip is clicked. */
export interface ClipSelection {
    kind: 'codaw.clip';
    track: string;
    index: number;
    clip: ProjectClip;
}

export namespace ClipSelection {
    export function is(selection: unknown): selection is ClipSelection {
        return typeof selection === 'object' && selection !== null && (selection as ClipSelection).kind === 'codaw.clip';
    }
}

/** A clip, with what the file alone does not say: lengths, the audio file, the signal path. */
export interface ClipProperties {
    name: string;
    trackId: string;
    trackFile: string;
    clipNumber: number;
    clip: ProjectClip;
    beatsPerBar: number;
    lengthBeats: number;
    lengthSeconds: number;
    audio?: { duration: number; repeats: number };
    audioError?: string;
    /** From the clip out to the master: each stage's gain, in dB. */
    signalPath: { stage: string; gain: number }[];
}

/** Supplies the Inspector with a clip's data when a clip is selected. */
@injectable()
export class ClipPropertyDataService implements PropertyDataService {

    readonly id = 'codaw.clip';
    readonly label = 'Clip';

    @inject(ProjectService)
    protected readonly projectService: ProjectService;

    @inject(AudioPeaksService)
    protected readonly audioPeaks: AudioPeaksService;

    canHandleSelection(selection: Object | undefined): number {
        return ClipSelection.is(selection) ? 100 : 0;
    }

    async providePropertyData(selection: Object | undefined): Promise<ClipProperties | undefined> {
        const project = this.projectService.project;
        if (!ClipSelection.is(selection) || !project) {
            return undefined;
        }
        const track = project.tracks.find(t => t.id === selection.track);
        const clip = selection.clip;
        const lengthBeats = clip.end - clip.start;
        const lengthSeconds = lengthBeats * 60 / project.bpm;
        const peaks = this.audioPeaks.get(clip.uri);

        const signalPath = [{ stage: 'clip', gain: clip.gain }];
        if (track) {
            signalPath.push({ stage: `track ${track.id}`, gain: track.gain });
            const bus = track.bus ? project.buses.find(b => b.id === track.bus) : undefined;
            if (bus) {
                signalPath.push({ stage: `bus ${bus.id}`, gain: bus.gain });
            }
        }
        if (project.master) {
            signalPath.push({ stage: 'master', gain: project.master.gain });
        }

        return {
            name: clip.file.split('/').pop() ?? clip.file,
            trackId: selection.track,
            trackFile: track?.file ?? '',
            clipNumber: selection.index + 1,
            clip,
            beatsPerBar: project.beatsPerBar,
            lengthBeats,
            lengthSeconds,
            audio: peaks && {
                duration: peaks.duration,
                // How often the audio plays in the clip: from the offset once, then from the start (as the engine loops).
                repeats: clip.loop
                    ? 1 + Math.max(0, (lengthSeconds - (peaks.duration - clip.offset)) / peaks.duration)
                    : Math.min(1, lengthSeconds / Math.max(0.001, peaks.duration - clip.offset))
            },
            audioError: this.audioPeaks.error(clip.uri),
            signalPath
        };
    }
}
