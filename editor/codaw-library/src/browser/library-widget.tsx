/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, codicon } from '@theia/core/lib/browser/widgets';
import { Message } from '@theia/core/lib/browser/widgets/widget';
import { MessageService } from '@theia/core/lib/common/message-service';
import { BinaryBuffer } from '@theia/core/lib/common/buffer';
import URI from '@theia/core/lib/common/uri';
import { FileService } from '@theia/filesystem/lib/browser/file-service';
import { FileStat } from '@theia/filesystem/lib/common/files';
import { ProjectService } from 'codaw-project/lib/browser/project-service';
import { AudioPeaksService } from 'codaw-project/lib/browser/audio-peaks-service';
import { SAMPLE_DRAG_MIME, DraggedSample, SampleDragService } from 'codaw-project/lib/browser/sample-drag';
import { CodawProject } from 'codaw-project/lib/common/project-model';
import { AudioPeaks } from 'codaw-project/lib/common/audio-peaks';
import { SamplePreview } from './sample-preview.js';

/** Where a project keeps its samples, relative to its root. */
export const SAMPLES_FOLDER = 'samples';

/** The audio files the engine reads. */
const AUDIO_EXTENSIONS = ['.wav', '.wave'];

/** How deep into `samples/` the Library looks. */
const MAX_DEPTH = 4;

export interface Sample {
    /** Relative to the project, as a clip's `file` says it: `samples/drums/kick.wav`. */
    file: string;
    uri: string;
    name: string;
    /** The folder within `samples/`, or "". */
    folder: string;
}

/**
 * The Library: the project's samples, the audio files in its `samples/` folder. Click one to hear it,
 * drag it onto a track to make a clip of it, and drop audio files from outside onto the Library to copy
 * them into the project.
 */
@injectable()
export class LibraryWidget extends ReactWidget {

    static readonly ID = 'codaw.library';
    static readonly LABEL = 'Library';
    static readonly ICON = 'library';

    @inject(ProjectService)
    protected readonly projectService: ProjectService;

    @inject(FileService)
    protected readonly fileService: FileService;

    @inject(AudioPeaksService)
    protected readonly audioPeaks: AudioPeaksService;

    @inject(SampleDragService)
    protected readonly sampleDrag: SampleDragService;

    @inject(SamplePreview)
    protected readonly preview: SamplePreview;

    @inject(MessageService)
    protected readonly messages: MessageService;

    protected samples: Sample[] = [];
    protected rootUri?: string;
    protected listing = false;
    /** Whether files from outside are being dragged over the Library. */
    protected dropActive = false;
    protected importing = false;
    protected readonly fileInput = document.createElement('input');

    @postConstruct()
    protected init(): void {
        this.id = LibraryWidget.ID;
        this.title.label = LibraryWidget.LABEL;
        this.title.caption = 'Library: the project\'s samples';
        this.title.iconClass = codicon(LibraryWidget.ICON);
        this.title.closable = true;
        this.addClass('codaw-library');
        this.node.tabIndex = 0;

        this.fileInput.type = 'file';
        this.fileInput.multiple = true;
        this.fileInput.accept = AUDIO_EXTENSIONS.join(',');
        this.fileInput.addEventListener('change', () => {
            const files = Array.from(this.fileInput.files ?? []);
            this.fileInput.value = '';
            this.importFiles(files);
        });

        this.toDispose.pushAll([
            this.projectService.onDidChange(project => this.onProject(project)),
            this.fileService.onDidFilesChange(event => {
                const folder = this.samplesUri;
                if (folder && event.changes.some(change => folder.isEqualOrParent(change.resource))) {
                    this.list();
                }
            }),
            this.audioPeaks.onDidChange(uri => {
                if (this.samples.some(sample => sample.uri === uri)) {
                    this.update();
                }
            }),
            this.preview.onDidChange(() => this.update())
        ]);
        this.onProject(this.projectService.project);
    }

    protected onActivateRequest(msg: Message): void {
        super.onActivateRequest(msg);
        this.node.focus();
    }

    protected get samplesUri(): URI | undefined {
        return this.rootUri ? new URI(this.rootUri).resolve(SAMPLES_FOLDER) : undefined;
    }

    protected onProject(project: CodawProject | undefined): void {
        if (project?.rootUri !== this.rootUri) {
            this.rootUri = project?.rootUri;
            this.preview.stop();
            this.list();
        } else {
            this.update();   // which samples are used may have changed
        }
    }

    // ── listing ───────────────────────────────────────────────────────────────────────────────────

    protected async list(): Promise<void> {
        const folder = this.samplesUri;
        const root = this.rootUri;
        if (!folder || !root) {
            this.samples = [];
            this.update();
            return;
        }
        this.listing = true;
        this.update();
        const found: Sample[] = [];
        try {
            if (await this.fileService.exists(folder)) {
                await this.collect(folder, new URI(root), folder, 0, found);
            }
        } catch (error) {
            console.warn('codaw: could not list the samples', error);
        }
        if (root !== this.rootUri) {
            return;   // another project was opened meanwhile; its listing wins
        }
        found.sort((a, b) => a.file.localeCompare(b.file, undefined, { numeric: true, sensitivity: 'base' }));
        this.samples = found;
        this.listing = false;
        this.update();
    }

    protected async collect(dir: URI, root: URI, samples: URI, depth: number, into: Sample[]): Promise<void> {
        const stat: FileStat = await this.fileService.resolve(dir);
        for (const child of stat.children ?? []) {
            if (child.isDirectory) {
                if (depth < MAX_DEPTH && !child.name.startsWith('.')) {
                    await this.collect(child.resource, root, samples, depth + 1, into);
                }
            } else if (isAudio(child.name)) {
                into.push({
                    file: root.relative(child.resource)?.toString() ?? child.name,
                    uri: child.resource.toString(),
                    name: child.name,
                    folder: samples.relative(child.resource.parent)?.toString() ?? ''
                });
            }
        }
    }

    // ── importing ─────────────────────────────────────────────────────────────────────────────────

    /** Opens the system's file picker to import samples. */
    pickFiles(): void {
        if (this.rootUri) {
            this.fileInput.click();
        }
    }

    /** Copies audio files into the project's `samples/`, under a new name if one is taken. */
    async importFiles(files: File[]): Promise<void> {
        const folder = this.samplesUri;
        if (!folder || !files.length) {
            return;
        }
        const audio = files.filter(file => isAudio(file.name));
        const skipped = files.filter(file => !isAudio(file.name));
        if (skipped.length) {
            this.messages.warn(`The Library takes WAV files only. Skipped: ${skipped.map(file => file.name).join(', ')}`);
        }
        if (!audio.length) {
            return;
        }
        this.importing = true;
        this.update();
        const imported: string[] = [];
        try {
            if (!await this.fileService.exists(folder)) {
                await this.fileService.createFolder(folder);
            }
            for (const file of audio) {
                const target = await this.freeName(folder, file.name);
                const bytes = new Uint8Array(await file.arrayBuffer());
                await this.fileService.writeFile(target, BinaryBuffer.wrap(bytes));
                imported.push(target.path.base);
            }
        } catch (error) {
            this.messages.error(`Could not import the samples: ${error instanceof Error ? error.message : error}`);
        } finally {
            this.importing = false;
            await this.list();
        }
        if (imported.length) {
            this.messages.info(`Imported ${imported.length === 1 ? imported[0] : `${imported.length} samples`} into ${SAMPLES_FOLDER}/.`);
        }
    }

    /** `name`, or `name 2`, `name 3`… if a file of that name is there already. */
    protected async freeName(folder: URI, name: string): Promise<URI> {
        const dot = name.lastIndexOf('.');
        const stem = dot > 0 ? name.slice(0, dot) : name;
        const extension = dot > 0 ? name.slice(dot) : '';
        let candidate = folder.resolve(name);
        for (let n = 2; await this.fileService.exists(candidate); n++) {
            candidate = folder.resolve(`${stem} ${n}${extension}`);
        }
        return candidate;
    }

    protected hasFiles(event: React.DragEvent): boolean {
        return event.dataTransfer.types.includes('Files') && !event.dataTransfer.types.includes(SAMPLE_DRAG_MIME);
    }

    protected readonly onDragOver = (event: React.DragEvent): void => {
        if (!this.rootUri || !this.hasFiles(event)) {
            return;
        }
        event.preventDefault();
        event.stopPropagation();
        event.dataTransfer.dropEffect = 'copy';
        if (!this.dropActive) {
            this.dropActive = true;
            this.update();
        }
    };

    protected readonly onDragLeave = (event: React.DragEvent): void => {
        if (this.dropActive && !this.node.contains(event.relatedTarget as Node | null)) {
            this.dropActive = false;
            this.update();
        }
    };

    protected readonly onDrop = (event: React.DragEvent): void => {
        if (!this.rootUri || !this.hasFiles(event)) {
            return;
        }
        event.preventDefault();
        event.stopPropagation();
        this.dropActive = false;
        this.importFiles(Array.from(event.dataTransfer.files));
    };

    // ── dragging samples out ──────────────────────────────────────────────────────────────────────

    protected startDrag(event: React.DragEvent, sample: Sample): void {
        const dragged: DraggedSample = { file: sample.file, uri: sample.uri, duration: this.audioPeaks.get(sample.uri)?.duration };
        event.dataTransfer.setData(SAMPLE_DRAG_MIME, JSON.stringify(dragged));
        event.dataTransfer.setData('text/plain', sample.file);
        event.dataTransfer.effectAllowed = 'copy';
        this.sampleDrag.current = dragged;
    }

    protected readonly endDrag = (): void => {
        this.sampleDrag.current = undefined;
    };

    // ── rendering ─────────────────────────────────────────────────────────────────────────────────

    protected render(): React.ReactNode {
        const project = this.projectService.project;
        if (!this.rootUri || !project) {
            return <div className='codaw-library-empty'>Open a project to see its samples.</div>;
        }
        const uses = usesOf(project);
        let folder: string | undefined;
        const rows: React.ReactNode[] = [];
        for (const sample of this.samples) {
            if (sample.folder !== folder) {
                folder = sample.folder;
                if (folder) {
                    rows.push(<div key={`folder:${folder}`} className='codaw-library-folder'>
                        <i className={codicon('folder')} /> {folder}
                    </div>);
                }
            }
            rows.push(this.renderSample(sample, project, uses.get(sample.file) ?? 0));
        }

        return <div className={`codaw-library-body${this.dropActive ? ' codaw-library-drop' : ''}`}
            onDragOver={this.onDragOver} onDragLeave={this.onDragLeave} onDrop={this.onDrop}>
            <div className='codaw-library-header'>
                <span className='codaw-library-title'>Samples</span>
                <span className='codaw-library-count'>{this.samples.length || ''}</span>
                <button className={`codaw-library-button ${codicon(this.importing ? 'loading' : 'add')}${this.importing ? ' codicon-modifier-spin' : ''}`}
                    title='Import Samples…' disabled={this.importing} onClick={() => this.pickFiles()} />
            </div>
            {this.samples.length
                ? <div className='codaw-library-list'>{rows}</div>
                : !this.listing && <div className='codaw-library-hint'>
                    <i className={codicon('cloud-upload')} />
                    <span>No samples yet. Drop WAV files here, or <a href='#' onClick={e => { e.preventDefault(); this.pickFiles(); }}>import them</a>.
                        They are copied into <code>{SAMPLES_FOLDER}/</code>.</span>
                </div>}
            {this.samples.length > 0 && <div className='codaw-library-footer'>
                Drag a sample onto a track to add it as a clip. Drop files here to import them.
            </div>}
        </div>;
    }

    protected renderSample(sample: Sample, project: CodawProject, uses: number): React.ReactNode {
        const peaks = this.audioPeaks.get(sample.uri);
        const error = this.audioPeaks.error(sample.uri);
        const playing = this.preview.playing === sample.uri;
        const meta: string[] = [];
        if (peaks) {
            meta.push(formatSeconds(peaks.duration));
            meta.push(formatBeats(peaks.duration * project.bpm / 60, project.beatsPerBar));
        }
        if (uses) {
            meta.push(`used ${uses}×`);
        }
        return <div key={sample.uri}
            className={`codaw-library-sample${playing ? ' codaw-library-playing' : ''}${error ? ' codaw-library-broken' : ''}`}
            title={error ? `${sample.file}: ${error}` : `${sample.file}\nClick to hear it, drag it onto a track to add it as a clip.`}
            draggable={!error}
            onDragStart={e => this.startDrag(e, sample)}
            onDragEnd={this.endDrag}
            onClick={() => this.preview.toggle(sample.uri)}>
            <i className={`codaw-library-play ${codicon(playing ? 'debug-stop' : 'play')}`} />
            <Waveform peaks={peaks} />
            <span className='codaw-library-names'>
                <span className='codaw-library-name'>{sample.name}</span>
                <span className='codaw-library-meta'>{error ? 'not readable' : meta.join(' · ')}</span>
            </span>
        </div>;
    }
}

const WAVEFORM_BARS = 40;

function Waveform({ peaks }: { peaks?: AudioPeaks }): React.ReactElement {
    if (!peaks || !peaks.duration) {
        return <svg className='codaw-library-wave' viewBox={`0 0 ${WAVEFORM_BARS} 20`} preserveAspectRatio='none' />;
    }
    const step = peaks.duration / WAVEFORM_BARS;
    let d = '';
    for (let i = 0; i < WAVEFORM_BARS; i++) {
        // Square root, as the arrangement draws them: quiet parts stay visible.
        const h = Math.max(0.5, Math.sqrt(AudioPeaks.between(peaks, i * step, (i + 1) * step)) * 9);
        d += `M${i + 0.5} ${10 - h}V${10 + h}`;
    }
    return <svg className='codaw-library-wave' viewBox={`0 0 ${WAVEFORM_BARS} 20`} preserveAspectRatio='none'>
        <path d={d} />
    </svg>;
}

function isAudio(name: string): boolean {
    const lower = name.toLowerCase();
    return AUDIO_EXTENSIONS.some(extension => lower.endsWith(extension));
}

/** How many clips use each file. */
function usesOf(project: CodawProject): Map<string, number> {
    const uses = new Map<string, number>();
    for (const track of project.tracks) {
        for (const clip of track.clips) {
            uses.set(clip.file, (uses.get(clip.file) ?? 0) + 1);
        }
    }
    return uses;
}

function formatSeconds(seconds: number): string {
    return seconds < 10 ? `${seconds.toFixed(2)} s` : seconds < 60 ? `${seconds.toFixed(1)} s`
        : `${Math.floor(seconds / 60)}:${String(Math.round(seconds % 60)).padStart(2, '0')}`;
}

function formatBeats(beats: number, beatsPerBar: number): string {
    const bars = beats / beatsPerBar;
    if (bars >= 0.98 && Math.abs(bars - Math.round(bars)) < 0.02) {
        const n = Math.round(bars);
        return `${n} bar${n === 1 ? '' : 's'}`;
    }
    const rounded = Math.round(beats * 10) / 10;
    return `${rounded} beat${rounded === 1 ? '' : 's'}`;
}
