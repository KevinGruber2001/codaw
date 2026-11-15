/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { Emitter, Event } from '@theia/core/lib/common/event';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { EngineStatus } from 'codaw-engine/lib/common/engine-protocol';
import { EngineService } from 'codaw-engine/lib/browser/engine-service';
import { ProjectService } from 'codaw-project/lib/browser/project-service';

export interface TransportState {
    readonly playing: boolean;
    readonly looping: boolean;
    /** The playhead, in beats from the start of the project. */
    readonly positionBeats: number;
    /** The project's tempo; unknown without a project. */
    readonly bpm?: number;
    readonly beatsPerBar: number;
    readonly beatUnit: number;
    readonly engine: EngineStatus;
    /** Why the engine is in its state, for `error`. */
    readonly engineMessage?: string;
}

/**
 * The transport as the transport bar and its commands see it. Playing, the playhead and the engine's
 * status come from the engine, through `EngineService`: they change when the engine says so, not when a
 * button is clicked, so the bar always shows what the engine is actually doing.
 *
 * Tempo and time signature are the project's, from project.toml's [transport].
 *
 * Loop is still local: the engine has no loop command yet.
 */
@injectable()
export class TransportService {

    @inject(EngineService)
    protected readonly engine: EngineService;

    @inject(ProjectService)
    protected readonly projectService: ProjectService;

    protected readonly onDidChangeEmitter = new Emitter<TransportState>();
    readonly onDidChange: Event<TransportState> = this.onDidChangeEmitter.event;

    protected looping = false;

    @postConstruct()
    protected init(): void {
        this.engine.onDidChangeStatus(() => this.fireChange());
        this.engine.onDidChangePosition(() => this.fireChange());
        this.projectService.onDidChange(() => this.fireChange());
    }

    get state(): TransportState {
        const project = this.projectService.project;
        return {
            playing: this.engine.position.playing,
            looping: this.looping,
            positionBeats: this.engine.position.beat,
            bpm: project?.bpm,
            beatsPerBar: project?.beatsPerBar ?? 4,
            beatUnit: project?.beatUnit ?? 4,
            engine: this.engine.status,
            engineMessage: this.engine.statusMessage
        };
    }

    /** The project's name, from project.toml's [project]; `undefined` without a project. */
    get projectName(): string | undefined {
        return this.projectService.project?.name;
    }

    /** project.toml, where tempo and time signature are set; `undefined` without a project. */
    get projectUri(): string | undefined {
        return this.projectService.project?.uri;
    }

    /** Whether there is an engine to command. */
    get ready(): boolean {
        return this.engine.status === 'running';
    }

    async togglePlay(): Promise<void> {
        if (this.engine.position.playing) {
            await this.engine.pause();
        } else {
            await this.engine.play();
        }
    }

    /** Stops playback and returns to the start, so the next play begins from the top. */
    async stop(): Promise<void> {
        await this.engine.pause();
        await this.engine.seek(0);
    }

    async returnToStart(): Promise<void> {
        await this.engine.seek(0);
    }

    /**
     * Moves the playhead whole bars: to the start of the bar `bars` away from the one it is in. Back from
     * the middle of a bar goes to that bar's start first, as rewinding does on a tape machine.
     */
    async stepBars(bars: number): Promise<void> {
        const { positionBeats, beatsPerBar } = this.state;
        const barStart = Math.floor(positionBeats / beatsPerBar) * beatsPerBar;
        const midBar = positionBeats - barStart > 0.01;
        const target = bars < 0 && midBar ? barStart + (bars + 1) * beatsPerBar : barStart + bars * beatsPerBar;
        await this.engine.seek(Math.max(0, target));
    }

    toggleLoop(): void {
        this.looping = !this.looping;
        this.fireChange();
    }

    protected fireChange(): void {
        this.onDidChangeEmitter.fire(this.state);
    }
}

/**
 * Formats a playhead position the way DAWs show it: `bar.beat.sixteenth`, each counted from 1.
 */
export function formatPosition(positionBeats: number, beatsPerBar: number): string {
    const bar = Math.floor(positionBeats / beatsPerBar) + 1;
    const beat = Math.floor(positionBeats % beatsPerBar) + 1;
    const sixteenth = Math.floor((positionBeats % 1) * 4) + 1;
    return `${bar}.${beat}.${sixteenth}`;
}
