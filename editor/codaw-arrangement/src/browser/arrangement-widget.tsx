/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, codicon } from '@theia/core/lib/browser/widgets';
import { Message } from '@theia/core/shared/@lumino/messaging';
import { SelectionService } from '@theia/core/lib/common/selection-service';
import { ArrangementView, HEADER_WIDTH, SampleDrop } from './arrangement-view.js';
import { Arrangement, ClipDrag, ClipRef, DragEdge, Snap, arrangementEnd, fromProject, snapBeat, snapStep } from './arrangement-model.js';
import { ProjectService } from 'codaw-project/lib/browser/project-service';
import { ProjectEditService } from 'codaw-project/lib/browser/project-edit-service';
import { ProjectCommands } from 'codaw-project/lib/browser/project-navigation';
import { TomlTarget } from 'codaw-project/lib/browser/toml-edits';
import { CommandService } from '@theia/core/lib/common/command';
import { ContextKey, ContextKeyService } from '@theia/core/lib/browser/context-key-service';
import { ArrangementClip } from './arrangement-model.js';
import { AudioPeaksService } from 'codaw-project/lib/browser/audio-peaks-service';
import { SchemaService } from 'codaw-project/lib/browser/schema-service';
import { DraggedSample, SAMPLE_DRAG_MIME, SampleDragService } from 'codaw-project/lib/browser/sample-drag';
import { CodawProject } from 'codaw-project/lib/common/project-model';
import { EnginePosition } from 'codaw-engine/lib/common/engine-protocol';
import { EngineService } from 'codaw-engine/lib/browser/engine-service';

/** The context key that is true while the arrangement has the keyboard. */
export const ARRANGEMENT_FOCUS = 'codawArrangementFocus';

/** The shortest a clip can be trimmed to, in beats. */
const MIN_CLIP_BEATS = 0.25;
const MIN_PX_PER_BEAT = 2;
const MAX_PX_PER_BEAT = 160;

/**
 * The arrangement: the project's tracks and clips on a timeline. The main view of CodaW.
 *
 * Holds the view's state and turns gestures into changes of it; `ArrangementView` draws it. The tracks and
 * clips are the open project's, read again whenever its files (or their open editors) change. A gesture
 * changes the view at once and the track's file through `ProjectEditService`; the re-read that follows
 * lands on the same values. The playhead is the engine's.
 */
@injectable()
export class ArrangementWidget extends ReactWidget {

    static readonly ID = 'codaw.arrangement';
    static readonly LABEL = 'Arrangement';
    static readonly ICON = 'layers';

    @inject(SelectionService)
    protected readonly selectionService: SelectionService;

    @inject(EngineService)
    protected readonly engine: EngineService;

    @inject(ProjectService)
    protected readonly projectService: ProjectService;

    @inject(AudioPeaksService)
    protected readonly audioPeaks: AudioPeaksService;

    @inject(SchemaService)
    protected readonly schemaService: SchemaService;

    @inject(ProjectEditService)
    protected readonly edits: ProjectEditService;

    @inject(SampleDragService)
    protected readonly sampleDrag: SampleDragService;

    @inject(CommandService)
    protected readonly commands: CommandService;

    @inject(ContextKeyService)
    protected readonly contextKeys: ContextKeyService;

    /** True while the arrangement has the keyboard: its keys (copy, paste, delete) apply only then. */
    protected focusKey: ContextKey<boolean>;

    /** The engine's last reported position, and when it arrived, to move the playhead on from. */
    protected lastPosition: EnginePosition = { beat: 0, playing: false };
    protected lastPositionAt = 0;
    protected animationFrame?: number;

    protected project?: CodawProject;
    protected arrangement: Arrangement = { bpm: 120, beatsPerBar: 4, beatUnit: 4, tracks: [] };
    protected pxPerBeat = 24;
    protected snap: Snap = 'beat';
    protected playhead = 0;
    protected selection?: ClipRef;
    /** The track pasting goes to: the selected clip's, or the one whose name was clicked last. */
    protected selectedTrack?: number;
    /** The clip copied last; pasted at the playhead. */
    protected clipboard?: ArrangementClip;
    protected drag?: ClipDrag;
    protected dropPreview?: SampleDrop;
    protected scrubbing = false;

    @postConstruct()
    protected init(): void {
        this.id = ArrangementWidget.ID;
        this.title.label = ArrangementWidget.LABEL;
        this.title.caption = ArrangementWidget.LABEL;
        this.title.iconClass = codicon(ArrangementWidget.ICON);
        this.title.closable = false;
        this.addClass('codaw-arrangement');
        this.node.tabIndex = 0;
        // Not through React: React registers wheel listeners as passive, and a passive listener cannot
        // stop the browser from zooming the whole page on Ctrl+wheel.
        this.node.addEventListener('wheel', this.onWheel, { passive: false });
        this.node.addEventListener('keydown', this.onKeyDown);
        // Cmd+C and Cmd+V, as the events they cause on the focused element. Whether the browser handles the
        // keys itself or Theia's Copy and Paste commands do (in Electron, by document.execCommand), these
        // events come here when the arrangement has the keyboard, and only then.
        this.node.addEventListener('copy', this.onCopy);
        this.node.addEventListener('paste', this.onPaste);
        this.focusKey = this.contextKeys.createKey<boolean>(ARRANGEMENT_FOCUS, false);
        this.node.addEventListener('focusin', () => this.focusKey.set(true));
        this.node.addEventListener('focusout', event => {
            if (!this.node.contains(event.relatedTarget as Node | null)) {
                this.focusKey.set(false);
            }
        });
        this.toDispose.push(this.engine.onDidChangePosition(position => this.onEnginePosition(position)));
        this.toDispose.push(this.projectService.onDidChange(project => this.setProject(project)));
        this.toDispose.push(this.audioPeaks.onDidChange(() => this.update()));
        this.setProject(this.projectService.project);
        this.toDispose.push({ dispose: () => this.stopAnimation() });
        this.onEnginePosition(this.engine.position);
    }

    protected onActivateRequest(msg: Message): void {
        super.onActivateRequest(msg);
        this.node.focus();
    }

    /** Shows a project, read anew. Which tracks are expanded is kept; a selection that no longer exists is dropped. */
    protected setProject(project: CodawProject | undefined): void {
        this.project = project;
        const expanded = new Set(this.arrangement.tracks.filter(t => t.expanded).map(t => t.id));
        if (project && this.arrangement.tracks.length === 0) {
            // The first time: open the tracks that have automation, so it is visible that it is there.
            project.tracks.filter(t => t.automation.length > 0).forEach(t => expanded.add(t.id));
        }
        this.arrangement = project ? fromProject(project, expanded, this.schemaService.schema) : { bpm: 120, beatsPerBar: 4, beatUnit: 4, tracks: [] };
        this.drag = undefined;
        if (this.selectedTrack !== undefined && !this.arrangement.tracks[this.selectedTrack]) {
            this.selectedTrack = undefined;
        }
        if (this.selection && !this.arrangement.tracks[this.selection.track]?.clips[this.selection.clip]) {
            this.select(undefined);
        }
        this.update();
    }

    protected render(): React.ReactNode {
        if (!this.project) {
            return <div className='codaw-arr-empty'>
                <i className={codicon('layers')} />
                <p>No CodaW project is open.</p>
                <p className='codaw-arr-empty-hint'>Open a folder with a <code>project.toml</code> (File → Open Folder).</p>
            </div>;
        }
        return <React.Fragment>
            {this.project.problems.length > 0 && <div className='codaw-arr-problems'>
                {this.project.problems.map((problem, i) => <div key={i}>
                    <i className={codicon('warning')} /> <b>{problem.file}</b>: {problem.message}
                </div>)}
            </div>}
            {this.renderArrangement()}
        </React.Fragment>;
    }

    protected renderArrangement(): React.ReactNode {
        return <ArrangementView
            arrangement={this.arrangement}
            pxPerBeat={this.pxPerBeat}
            snap={this.snap}
            playhead={this.playhead}
            selection={this.selection}
            selectedTrack={this.selectedTrack}
            drag={this.drag}
            dropPreview={this.dropPreview}
            onLaneDragOver={this.onLaneDragOver}
            onLaneDragLeave={this.onLaneDragLeave}
            onLaneDrop={this.onLaneDrop}
            onSnapChange={this.setSnap}
            onRulerPointerDown={this.onRulerPointerDown}
            onRulerPointerMove={this.onRulerPointerMove}
            onRulerPointerUp={this.onRulerPointerUp}
            onClipPointerDown={this.onClipPointerDown}
            onClipPointerMove={this.onClipPointerMove}
            onClipPointerUp={this.onClipPointerUp}
            onToggleMute={this.toggleMute}
            onToggleSolo={this.toggleSolo}
            onToggleExpanded={this.toggleExpanded}
            onBackgroundPointerDown={this.clearSelection}
            audioOf={this.audioOf}
            audioErrorOf={this.audioErrorOf}
            isDirty={this.isDirty}
            onZoom={this.onZoom}
            onRevealTrack={this.revealTrack}
            onRevealAutomation={this.revealAutomation}
        />;
    }

    protected readonly audioOf = (uri: string) => this.audioPeaks.get(uri);
    protected readonly audioErrorOf = (uri: string) => this.audioPeaks.error(uri);
    protected readonly isDirty = (uri: string) => this.projectService.isDirty(uri);

    protected readonly onZoom = (zoom: 'in' | 'out' | 'fit'): void => {
        if (zoom === 'fit') {
            this.zoomToFit();
        } else {
            this.zoomBy(zoom === 'in' ? 1.25 : 0.8);
        }
    };

    // ── showing where things are defined ──────────────────────────────────────────────────────────

    /** Opens a track's file next to the arrangement, at a section of it (the top, without one). */
    protected reveal(trackIndex: number, target?: TomlTarget): void {
        const track = this.arrangement.tracks[trackIndex];
        if (track) {
            this.commands.executeCommand(ProjectCommands.REVEAL_IN_FILE.id, track.uri, target);
        }
    }

    protected readonly revealTrack = (track: number): void => {
        this.selectedTrack = track;
        this.update();
        this.reveal(track);
    };
    protected readonly revealAutomation = (track: number, lane: number): void =>
        this.reveal(track, { section: 'automation', index: lane });

    // ── zoom ──────────────────────────────────────────────────────────────────────────────────────

    /** Zooms by a factor, keeping the beat under `clientX` (or the middle of the view) where it is. */
    zoomBy(factor: number, clientX?: number): void {
        const scroller = this.scroller;
        const next = Math.min(MAX_PX_PER_BEAT, Math.max(MIN_PX_PER_BEAT, this.pxPerBeat * factor));
        if (!scroller || next === this.pxPerBeat) {
            this.setZoom(next);
            return;
        }
        const rect = scroller.getBoundingClientRect();
        const anchorX = (clientX ?? rect.left + HEADER_WIDTH + (rect.width - HEADER_WIDTH) / 2) - rect.left - HEADER_WIDTH;
        const anchorBeat = (scroller.scrollLeft + anchorX) / this.pxPerBeat;
        this.setZoom(next, () => scroller.scrollLeft = anchorBeat * next - anchorX);
    }

    /** Zooms so that everything from the first bar to the last clip fits the width of the view. */
    zoomToFit(): void {
        const scroller = this.scroller;
        if (!scroller) {
            return;
        }
        const end = Math.max(arrangementEnd(this.arrangement), this.arrangement.beatsPerBar * 4);
        const available = scroller.clientWidth - HEADER_WIDTH - 24;
        this.setZoom(Math.min(MAX_PX_PER_BEAT, Math.max(MIN_PX_PER_BEAT, available / end)), () => scroller.scrollLeft = 0);
    }

    protected setZoom(pxPerBeat: number, thenScroll?: () => void): void {
        this.pxPerBeat = pxPerBeat;
        this.update();
        // React renders on its own schedule; the new width is only there to scroll in on the next frame.
        if (thenScroll) {
            requestAnimationFrame(thenScroll);
        }
    }

    protected get scroller(): HTMLElement | null {
        return this.node.querySelector<HTMLElement>('.codaw-arr-scroller');
    }

    protected readonly onWheel = (event: WheelEvent): void => {
        if (event.ctrlKey || event.metaKey) {
            event.preventDefault();
            this.zoomBy(event.deltaY < 0 ? 1.15 : 1 / 1.15, event.clientX);
        }
    };

    // ── following the engine ──────────────────────────────────────────────────────────────────────

    /**
     * The engine reports the playhead about 10 times a second; drawn at that rate it would jump. So while
     * playing, the playhead moves on from the last report at the project's tempo, every frame, and each
     * new report corrects it.
     */
    protected onEnginePosition(position: EnginePosition): void {
        this.lastPosition = position;
        this.lastPositionAt = performance.now();
        if (position.playing) {
            this.startAnimation();
        } else {
            this.stopAnimation();
            this.playhead = position.beat;
            this.update();
        }
    }

    protected startAnimation(): void {
        if (this.animationFrame !== undefined) {
            return;
        }
        const frame = () => {
            const elapsedSeconds = (performance.now() - this.lastPositionAt) / 1000;
            this.playhead = this.lastPosition.beat + elapsedSeconds * this.arrangement.bpm / 60;
            this.movePlayhead();
            this.animationFrame = requestAnimationFrame(frame);
        };
        this.animationFrame = requestAnimationFrame(frame);
    }

    protected stopAnimation(): void {
        if (this.animationFrame !== undefined) {
            cancelAnimationFrame(this.animationFrame);
            this.animationFrame = undefined;
        }
    }

    /** Moves the playhead without re-rendering the arrangement: 60 times a second, only it changes. */
    protected movePlayhead(): void {
        const line = this.node.querySelector<HTMLElement>('.codaw-arr-playhead');
        const marker = this.node.querySelector<HTMLElement>('.codaw-arr-playhead-marker');
        if (line) {
            line.style.left = `${HEADER_WIDTH + this.playhead * this.pxPerBeat}px`;
        }
        if (marker) {
            marker.style.left = `${this.playhead * this.pxPerBeat}px`;
        }
    }

    // ── snap and playhead ─────────────────────────────────────────────────────────────────────────

    protected readonly setSnap = (snap: Snap): void => {
        this.snap = snap;
        this.update();
    };

    protected get step(): number {
        return snapStep(this.snap, this.arrangement.beatsPerBar);
    }

    protected readonly onRulerPointerDown = (event: React.PointerEvent<HTMLElement>): void => {
        event.stopPropagation();
        event.currentTarget.setPointerCapture(event.pointerId);
        this.scrubbing = true;
        this.seekTo(event);
    };

    protected readonly onRulerPointerMove = (event: React.PointerEvent<HTMLElement>): void => {
        if (this.scrubbing) {
            this.seekTo(event);
        }
    };

    protected readonly onRulerPointerUp = (): void => {
        this.scrubbing = false;
    };

    protected seekTo(event: React.PointerEvent<HTMLElement>): void {
        const beat = (event.clientX - event.currentTarget.getBoundingClientRect().left) / this.pxPerBeat;
        // The playhead snaps to the grid too, but never coarser than a beat: seeking by bars is annoying.
        this.playhead = Math.max(0, snapBeat(beat, Math.min(this.step || 0, 1)));
        this.update();
        if (this.engine.status === 'running') {
            // The engine answers with its new position, which is where the playhead then stays.
            this.engine.seek(this.playhead).catch(() => { /* reported in the engine log */ });
        }
    }

    // ── clips: select, move, trim ─────────────────────────────────────────────────────────────────

    protected readonly onClipPointerDown = (event: React.PointerEvent<HTMLElement>, ref: ClipRef, edge: DragEdge): void => {
        // Only the innermost handler: an edge sits inside its clip, and both get the event.
        if (event.button !== 0 || this.drag) {
            event.stopPropagation();
            return;
        }
        event.stopPropagation();
        event.currentTarget.setPointerCapture(event.pointerId);
        const clip = this.arrangement.tracks[ref.track].clips[ref.clip];
        this.select(ref);
        this.drag = {
            ref, edge, pointerStartX: event.clientX,
            originalStart: clip.start, originalEnd: clip.end, start: clip.start, end: clip.end
        };
        this.update();
    };

    protected readonly onClipPointerMove = (event: React.PointerEvent<HTMLElement>): void => {
        const drag = this.drag;
        if (!drag) {
            return;
        }
        const delta = (event.clientX - drag.pointerStartX) / this.pxPerBeat;
        // Alt drags freely, off the grid.
        const step = event.altKey ? 0 : this.step;
        const length = drag.originalEnd - drag.originalStart;
        switch (drag.edge) {
            case 'move':
                drag.start = Math.max(0, snapBeat(drag.originalStart + delta, step));
                drag.end = drag.start + length;
                break;
            case 'start':
                drag.start = Math.min(drag.originalEnd - MIN_CLIP_BEATS, Math.max(0, snapBeat(drag.originalStart + delta, step)));
                break;
            case 'end':
                drag.end = Math.max(drag.originalStart + MIN_CLIP_BEATS, snapBeat(drag.originalEnd + delta, step));
                break;
        }
        this.update();
    };

    protected readonly onClipPointerUp = (): void => {
        const drag = this.drag;
        if (!drag) {
            return;
        }
        const track = this.arrangement.tracks[drag.ref.track];
        const clip = track.clips[drag.ref.clip];
        const changed = drag.start !== drag.originalStart || drag.end !== drag.originalEnd;
        // Trimming the start of a one-shot clip moves its in-point, so the audio stays where it was.
        const trimsInPoint = drag.edge === 'start' && !clip.loop;
        if (trimsInPoint) {
            clip.offset = Math.max(0, clip.offset + (drag.start - drag.originalStart) * 60 / this.arrangement.bpm);
        }
        // Right away in the view, so the clip does not jump back while the file is changed and read again.
        clip.start = drag.start;
        clip.end = drag.end;
        this.drag = undefined;
        this.select(drag.ref);
        this.update();

        if (!changed) {
            // A click, not a drag: show the clip in its file.
            this.reveal(drag.ref.track, { section: 'clip', index: drag.ref.clip });
        } else {
            if (trimsInPoint) {
                this.edits.trimClip(track.uri, drag.ref.clip, clip.start, clip.end, clip.offset);
            } else {
                this.edits.moveClip(track.uri, drag.ref.clip, clip.start, clip.end);
            }
        }
    };

    protected select(ref: ClipRef | undefined): void {
        this.selection = ref;
        if (ref) {
            this.selectedTrack = ref.track;
        }
        // The application-wide selection, for the inspector to show what is selected here.
        this.selectionService.selection = ref && {
            kind: 'codaw.clip',
            track: this.arrangement.tracks[ref.track].id,
            index: ref.clip,
            clip: { ...this.arrangement.tracks[ref.track].clips[ref.clip] }
        };
    }

    protected readonly clearSelection = (): void => {
        if (this.selection) {
            this.select(undefined);
            this.update();
        }
    };

    // ── copy, paste, delete ───────────────────────────────────────────────────────────────────────

    /** Whether the arrangement has the keyboard. */
    get hasFocus(): boolean {
        return this.node.contains(document.activeElement);
    }

    get canCopy(): boolean {
        return !!this.selection;
    }

    get canPaste(): boolean {
        return !!this.clipboard && this.selectedTrack !== undefined && !!this.arrangement.tracks[this.selectedTrack];
    }

    /** Copies the selected clip, and puts it on the system clipboard as a [[clip]] table. */
    copyToClipboard(): void {
        const toml = this.copySelection();
        if (toml !== undefined) {
            navigator.clipboard?.writeText(toml).catch(() => { /* no clipboard access: pasting here still works */ });
        }
    }

    protected readonly onCopy = (event: ClipboardEvent): void => {
        const toml = this.copySelection();
        if (toml !== undefined) {
            event.preventDefault();
            event.clipboardData?.setData('text/plain', toml);
        }
    };

    protected readonly onPaste = (event: ClipboardEvent): void => {
        if (this.canPaste) {
            event.preventDefault();
            this.paste();
        }
    };

    /**
     * Copies the selected clip, for pasting here; returns it as a [[clip]] table, which the copy event
     * puts on the system clipboard (so it can be pasted into a file as text, too).
     */
    copySelection(): string | undefined {
        const ref = this.selection;
        if (!ref) {
            return undefined;
        }
        const clip = { ...this.arrangement.tracks[ref.track].clips[ref.clip] };
        this.clipboard = clip;
        const toml = ['[[clip]]', `file   = "${clip.file}"`, `start  = ${clip.start}`, `end    = ${clip.end}`,
            `loop   = ${clip.loop}`, `gain   = ${clip.gain}`, `offset = ${clip.offset}`].join('\n');
        return toml;
    }

    /**
     * Pastes the copied clip on the selected track, starting at the playhead (on the snap grid), as a new
     * [[clip]] after the track's last one. The new clip is selected, so it can be copied on in turn.
     */
    paste(): void {
        const clip = this.clipboard;
        const index = this.selectedTrack;
        if (!clip || index === undefined) {
            return;
        }
        const track = this.arrangement.tracks[index];
        if (!track) {
            return;
        }
        const start = Math.max(0, snapBeat(this.playhead, this.step));
        const pasted: ArrangementClip = { ...clip, start, end: start + (clip.end - clip.start) };
        // In the view at once, as the last clip, which is where it goes in the file too.
        track.clips.push(pasted);
        this.select({ track: index, clip: track.clips.length - 1 });
        this.update();
        this.edits.addClip(track.uri, pasted);
    }

    /** Deletes the selected clip from its track's file. */
    deleteSelection(): void {
        const ref = this.selection;
        if (!ref) {
            return;
        }
        const track = this.arrangement.tracks[ref.track];
        track.clips.splice(ref.clip, 1);
        this.select(undefined);
        this.update();
        this.edits.removeClip(track.uri, ref.clip);
    }

    protected readonly onKeyDown = (event: KeyboardEvent): void => {
        // Cmd+C / Cmd+V where nothing else took them (in a browser, Theia binds no copy or paste of its own
        // to an element that is not text). In Electron, Theia's Copy and Paste take the keys first and come
        // here through the handlers in ArrangementContribution instead.
        if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey) {
            const key = event.key.toLowerCase();
            if (key === 'c' && this.canCopy) {
                event.preventDefault();
                this.copyToClipboard();
                return;
            }
            if (key === 'v' && this.canPaste) {
                event.preventDefault();
                this.paste();
                return;
            }
        }
        if (event.key === 'Escape') {
            this.clearSelection();
        }
    };

    // ── samples dropped from the Library ──────────────────────────────────────────────────────────

    /**
     * Where a sample would land on a track: at the pointer, on the snap grid (Alt ignores it), as long as
     * the sample plays at the project's tempo, or a bar if that is not known yet.
     */
    protected sampleDrop(event: React.DragEvent<HTMLElement>, track: number, sample: DraggedSample): SampleDrop {
        const beat = (event.clientX - event.currentTarget.getBoundingClientRect().left) / this.pxPerBeat;
        const start = Math.max(0, snapBeat(beat, event.altKey ? 0 : this.step));
        const duration = sample.duration ?? this.audioPeaks.get(sample.uri)?.duration;
        const beats = duration ? duration * this.arrangement.bpm / 60 : this.arrangement.beatsPerBar;
        return { track, file: sample.file, start, end: Math.round((start + beats) * 1000) / 1000 };
    }

    protected readonly onLaneDragOver = (event: React.DragEvent<HTMLElement>, track: number): void => {
        if (!event.dataTransfer.types.includes(SAMPLE_DRAG_MIME)) {
            return;
        }
        // Theia's own listeners further up (the dock panel's, the document's) set the drop effect to
        // "link" or "none", which the Library's "copy" does not allow, and the drop would be refused.
        event.preventDefault();
        event.stopPropagation();
        event.dataTransfer.dropEffect = 'copy';
        const sample = this.sampleDrag.current;
        const preview = sample && this.sampleDrop(event, track, sample);
        const old = this.dropPreview;
        if (preview?.track !== old?.track || preview?.start !== old?.start || preview?.end !== old?.end) {
            this.dropPreview = preview;
            this.update();
        }
    };

    protected readonly onLaneDragLeave = (event: React.DragEvent<HTMLElement>, track: number): void => {
        if (this.dropPreview?.track === track && !event.currentTarget.contains(event.relatedTarget as Node | null)) {
            this.dropPreview = undefined;
            this.update();
        }
    };

    /** Adds the dropped sample to the track as a new clip, after its last one, and selects it. */
    protected readonly onLaneDrop = (event: React.DragEvent<HTMLElement>, index: number): void => {
        const data = event.dataTransfer.getData(SAMPLE_DRAG_MIME);
        this.dropPreview = undefined;
        if (!data) {
            this.update();
            return;
        }
        event.preventDefault();
        event.stopPropagation();   // not for the dock panel, which would try to open it as a file
        const track = this.arrangement.tracks[index];
        let sample: DraggedSample;
        try {
            sample = JSON.parse(data);
        } catch {
            this.update();
            return;
        }
        const { start, end } = this.sampleDrop(event, index, sample);
        const clip: ArrangementClip = { file: sample.file, uri: sample.uri, start, end, loop: false, gain: 0, offset: 0 };
        track.clips.push(clip);
        this.select({ track: index, clip: track.clips.length - 1 });
        this.update();
        this.node.focus();
        this.edits.addClip(track.uri, clip);
    };

    // ── tracks ────────────────────────────────────────────────────────────────────────────────────

    protected readonly toggleMute = (index: number): void => {
        const track = this.arrangement.tracks[index];
        track.mute = !track.mute;
        this.update();
        this.edits.setMute(track.uri, track.mute);
    };

    protected readonly toggleSolo = (index: number): void => {
        const track = this.arrangement.tracks[index];
        track.solo = !track.solo;
        this.update();
        this.edits.setSolo(track.uri, track.solo);
    };

    protected readonly toggleExpanded = (track: number): void => {
        this.arrangement.tracks[track].expanded = !this.arrangement.tracks[track].expanded;
        this.update();
    };
}
