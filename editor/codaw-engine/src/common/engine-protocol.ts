export const ENGINE_SERVICE_PATH = '/services/codaw-engine';

export type EngineStatus = 'offline' | 'starting' | 'running' | 'error';

export interface EnginePosition { beat: number; playing: boolean; }

export interface EngineStartOptions {
    /** The `codaw` binary. */
    enginePath: string;
    /** Absolute path of the project's project.toml. */
    projectFile: string;
}

/** Backend → frontend. Implemented in the browser, called by the backend through a proxy. */
export interface EngineClient {
    onStatus(status: EngineStatus, message?: string): void;
    onPosition(position: EnginePosition): void;
    onLog(line: string): void;
}

/** Frontend → backend. Implemented in node, called by the browser through a proxy. */
export const EngineServer = Symbol('EngineServer');
export interface EngineServer {
    start(options: EngineStartOptions): Promise<void>;
    stop(): Promise<void>;          // shuts the process down
    play(): Promise<void>;
    pause(): Promise<void>;         // the protocol's "stop" = pause at the playhead
    seek(beat: number): Promise<void>;
    /** What `codaw schema` prints: what project files may contain. Runs the binary once; needs no project. */
    schema(enginePath: string): Promise<unknown>;
    setClient(client: EngineClient | undefined): void;
    dispose(): void;
}