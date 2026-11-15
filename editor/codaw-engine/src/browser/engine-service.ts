/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { Emitter, Event } from '@theia/core/lib/common/event';
import { PreferenceService } from '@theia/core/lib/common/preferences';
import { RemoteConnectionProvider, ServiceConnectionProvider } from '@theia/core/lib/browser/messaging/service-connection-provider';
import { ENGINE_SERVICE_PATH, EngineClient, EnginePosition, EngineServer, EngineStatus } from '../common/engine-protocol.js';
import { ENGINE_PATH_PREFERENCE } from './engine-preferences.js';

/** How many log lines are kept for a log view opened late. Older lines are dropped. */
const MAX_LOG_LINES = 1000;

export interface EngineStatusChange {
    status: EngineStatus;
    /** Why, for `error`; otherwise usually absent. */
    message?: string;
}

/**
 * The audio engine, as the rest of the frontend sees it: its status, the playhead, its log, and the
 * commands to drive it. The one class features use; the RPC to the backend stays behind it.
 *
 * It is also the `EngineClient`: the backend's calls (`onStatus`, `onPosition`, `onLog`) land on this
 * object. Each of them does the same two things: remember the new state, for whoever asks later, and
 * announce it, for whoever listens now.
 */
@injectable()
export class EngineService implements EngineClient {

    @inject(RemoteConnectionProvider)
    protected readonly connectionProvider: ServiceConnectionProvider;

    @inject(PreferenceService)
    protected readonly preferenceService: PreferenceService;

    /** The backend's `EngineServerImpl`, through a proxy. */
    protected server: EngineServer;

    protected _status: EngineStatus = 'offline';
    protected _statusMessage?: string;
    protected _position: EnginePosition = { beat: 0, playing: false };
    protected readonly _log: string[] = [];

    protected readonly onDidChangeStatusEmitter = new Emitter<EngineStatusChange>();
    readonly onDidChangeStatus: Event<EngineStatusChange> = this.onDidChangeStatusEmitter.event;

    protected readonly onDidChangePositionEmitter = new Emitter<EnginePosition>();
    readonly onDidChangePosition: Event<EnginePosition> = this.onDidChangePositionEmitter.event;

    protected readonly onDidLogEmitter = new Emitter<string>();
    readonly onDidLog: Event<string> = this.onDidLogEmitter.event;

    @postConstruct()
    protected init(): void {
        // The proxy is created here rather than injected: it needs this object as its client, and an
        // injected proxy built with this service as its client would be a dependency cycle.
        this.server = this.connectionProvider.createProxy<EngineServer>(ENGINE_SERVICE_PATH, this);
    }

    // ── the state, for whoever asks ─────────────────────────────────────────────────────────────

    get status(): EngineStatus {
        return this._status;
    }

    get statusMessage(): string | undefined {
        return this._statusMessage;
    }

    get position(): EnginePosition {
        return this._position;
    }

    /** The most recent log lines, oldest first. */
    get log(): readonly string[] {
        return this._log;
    }

    // ── EngineClient: called by the backend ─────────────────────────────────────────────────────
    //
    // These are called by the RPC layer: they must not throw, and they must not call back into the
    // server. Reacting to a change is the listeners' job.

    onStatus(status: EngineStatus, message?: string): void {
        this._status = status;
        this._statusMessage = message;
        this.appendLog(message ? `[engine] ${status}: ${message}` : `[engine] ${status}`);

        // A stopped or dead engine sends no last position, so it is not playing anymore: say so,
        // or the play button would stay on "playing" forever.
        if ((status === 'offline' || status === 'error') && this._position.playing) {
            this.onPosition({ beat: this._position.beat, playing: false });
        }
        this.onDidChangeStatusEmitter.fire({ status, message });
    }

    onPosition(position: EnginePosition): void {
        // ~10 times a second while playing: keep this cheap.
        this._position = position;
        this.onDidChangePositionEmitter.fire(position);
    }

    onLog(line: string): void {
        this.appendLog(line);
    }

    protected appendLog(line: string): void {
        const stamped = `${new Date().toLocaleTimeString()}  ${line}`;
        this._log.push(stamped);
        if (this._log.length > MAX_LOG_LINES) {
            this._log.splice(0, this._log.length - MAX_LOG_LINES);
        }
        this.onDidLogEmitter.fire(stamped);
    }

    // ── commands: what the rest of the frontend calls ───────────────────────────────────────────

    /** Starts the engine on a project, replacing a running one. The outcome arrives as status changes. */
    async start(projectFile: string): Promise<void> {
        await this.preferenceService.ready;
        const enginePath = this.preferenceService.get<string>(ENGINE_PATH_PREFERENCE, 'codaw');
        await this.server.start({ enginePath, projectFile });
    }

    /** What project files may contain, as the engine binary itself says (`codaw schema`). */
    async schema(): Promise<unknown> {
        await this.preferenceService.ready;
        return this.server.schema(this.preferenceService.get<string>(ENGINE_PATH_PREFERENCE, 'codaw'));
    }

    stop(): Promise<void> {
        return this.server.stop();
    }

    play(): Promise<void> {
        return this.server.play();
    }

    pause(): Promise<void> {
        return this.server.pause();
    }

    seek(beat: number): Promise<void> {
        return this.server.seek(beat);
    }
}
