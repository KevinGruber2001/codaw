/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { Emitter, Event } from '@theia/core/lib/common/event';
import URI from '@theia/core/lib/common/uri';
import { FileService } from '@theia/filesystem/lib/browser/file-service';
import { AudioPeaks, decodeWavPeaks } from '../common/audio-peaks.js';

type Entry = { peaks: AudioPeaks } | { loading: true } | { error: string };

/**
 * The peaks of the project's audio files, for drawing waveforms. Each file is read once, in the
 * background, and read again when it changes on disk.
 */
@injectable()
export class AudioPeaksService {

    @inject(FileService)
    protected readonly fileService: FileService;

    protected readonly entries = new Map<string, Entry>();

    protected readonly onDidChangeEmitter = new Emitter<string>();
    /** Fires with a file's URI when its peaks are ready, or changed. */
    readonly onDidChange: Event<string> = this.onDidChangeEmitter.event;

    @postConstruct()
    protected init(): void {
        this.fileService.onDidFilesChange(event => {
            for (const uri of this.entries.keys()) {
                if (event.contains(new URI(uri))) {
                    this.entries.delete(uri);
                    this.onDidChangeEmitter.fire(uri);   // listeners ask again, which reads the new file
                }
            }
        });
    }

    /**
     * The peaks of an audio file, if they are ready. If not, starts reading it and returns `undefined`;
     * `onDidChange` fires when they are.
     */
    get(uri: string): AudioPeaks | undefined {
        const entry = this.entries.get(uri);
        if (!entry) {
            this.load(uri);
            return undefined;
        }
        return 'peaks' in entry ? entry.peaks : undefined;
    }

    /** Why a file has no peaks, if reading it failed. */
    error(uri: string): string | undefined {
        const entry = this.entries.get(uri);
        return entry && 'error' in entry ? entry.error : undefined;
    }

    protected async load(uri: string): Promise<void> {
        this.entries.set(uri, { loading: true });
        let entry: Entry;
        try {
            const content = await this.fileService.readFile(new URI(uri));
            entry = { peaks: decodeWavPeaks(content.value.buffer) };
        } catch (error) {
            entry = { error: error instanceof Error ? error.message : String(error) };
        }
        this.entries.set(uri, entry);
        this.onDidChangeEmitter.fire(uri);
    }
}
