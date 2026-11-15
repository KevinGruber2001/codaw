/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable } from '@theia/core/shared/inversify';
import { Emitter, Event } from '@theia/core/lib/common/event';
import URI from '@theia/core/lib/common/uri';
import { FileService } from '@theia/filesystem/lib/browser/file-service';

/**
 * Plays a sample to hear what it is, in the browser through Web Audio. The engine is not involved: a
 * preview neither changes the project nor stops its playback. One sample at a time.
 */
@injectable()
export class SamplePreview {

    @inject(FileService)
    protected readonly fileService: FileService;

    protected context?: AudioContext;
    protected source?: AudioBufferSourceNode;
    /** Decoded samples, by URI. Dropped when the file changes on disk. */
    protected readonly buffers = new Map<string, Promise<AudioBuffer>>();

    protected readonly onDidChangeEmitter = new Emitter<void>();
    /** Fires when a preview starts or stops. */
    readonly onDidChange: Event<void> = this.onDidChangeEmitter.event;

    /** The sample playing now, if any. */
    playing?: string;

    toggle(uri: string): void {
        if (this.playing === uri) {
            this.stop();
        } else {
            this.play(uri);
        }
    }

    async play(uri: string): Promise<void> {
        this.stop();
        this.playing = uri;
        this.onDidChangeEmitter.fire();
        try {
            const context = this.context ??= new AudioContext();
            this.listenForChanges();
            const buffer = await this.decode(context, uri);
            if (this.playing !== uri) {
                return;   // stopped, or another sample started, while this one was loading
            }
            await context.resume();
            const source = context.createBufferSource();
            source.buffer = buffer;
            source.connect(context.destination);
            source.onended = () => {
                if (this.source === source) {
                    this.source = undefined;
                    this.playing = undefined;
                    this.onDidChangeEmitter.fire();
                }
            };
            this.source = source;
            source.start();
        } catch (error) {
            console.warn(`codaw: could not play ${uri}`, error);
            this.buffers.delete(uri);
            if (this.playing === uri) {
                this.playing = undefined;
                this.onDidChangeEmitter.fire();
            }
        }
    }

    stop(): void {
        const source = this.source;
        this.source = undefined;
        source?.stop();
        if (this.playing) {
            this.playing = undefined;
            this.onDidChangeEmitter.fire();
        }
    }

    protected decode(context: AudioContext, uri: string): Promise<AudioBuffer> {
        let buffer = this.buffers.get(uri);
        if (!buffer) {
            buffer = this.fileService.readFile(new URI(uri)).then(content => {
                const bytes = content.value.buffer;
                // decodeAudioData takes the ArrayBuffer whole, so give it a copy of exactly these bytes.
                return context.decodeAudioData(bytes.slice().buffer as ArrayBuffer);
            });
            this.buffers.set(uri, buffer);
        }
        return buffer;
    }

    protected listening = false;

    protected listenForChanges(): void {
        if (this.listening) {
            return;
        }
        this.listening = true;
        this.fileService.onDidFilesChange(event => {
            for (const uri of [...this.buffers.keys()]) {
                if (event.contains(new URI(uri))) {
                    this.buffers.delete(uri);
                }
            }
        });
    }
}
