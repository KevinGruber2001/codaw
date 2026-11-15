import { injectable } from '@theia/core/shared/inversify';
import { ChildProcessWithoutNullStreams, execFile, spawn } from 'child_process';
import * as path from 'path';
import * as readline from 'readline';
import { EngineClient, EngineServer, EngineStartOptions } from '../common/engine-protocol.js';
import { EngineCommand, EngineCommandType, EngineMessage, EngineMessageType } from './engine-messages.js';

/** The protocol revision this server speaks; must match `ProtocolVersion` in internal/serve/serve.go. */
const PROTOCOL_VERSION = 1;
/** How long a fresh engine may take to say hello before we give up on it. */
const HELLO_TIMEOUT_MS = 10_000;
/** How long a stopping engine gets to exit by itself before it is killed. */
const SHUTDOWN_GRACE_MS = 3_000;
/** How long `codaw schema` may take: it only prints a table. */
const SCHEMA_TIMEOUT_MS = 10_000;

/**
 * Runs one `codaw serve` process for one frontend connection, and translates between the two:
 * RPC calls from the frontend become lines on the engine's stdin, lines from its stdout become
 * calls on the frontend's `EngineClient`.
 */
@injectable()
export class EngineServerImpl implements EngineServer {

    protected client?: EngineClient;
    protected child?: ChildProcessWithoutNullStreams;
    protected helloTimer?: ReturnType<typeof setTimeout>;

    setClient(client: EngineClient | undefined): void {
        this.client = client;
    }

    // ── lifecycle ───────────────────────────────────────────────────────────────────────────

    async start({ enginePath, projectFile }: EngineStartOptions): Promise<void> {
        // One engine per connection: starting again replaces the running one.
        if (this.child) {
            await this.stop();
        }
        this.client?.onStatus('starting');

        const child = spawn(enginePath, ['serve', projectFile], { cwd: path.dirname(projectFile) });
        this.child = child;

        // stdout is the protocol, one JSON object per line. stderr is the engine's log.
        readline.createInterface({ input: child.stdout }).on('line', line => this.onLine(child, line));
        readline.createInterface({ input: child.stderr }).on('line', line => this.client?.onLog(line));

        // The process could not be started at all, e.g. there is no binary at `enginePath`.
        child.on('error', error => this.onGone(child, `Could not start the engine (${enginePath}): ${error.message}`));
        // The process ended without us stopping it: it crashed, or exited on an error of its own.
        child.on('exit', (code, signal) => this.onGone(child, `The engine exited (${signal ?? `code ${code}`}).`));
        // Writing to an engine that just died fails asynchronously, as an 'error' on stdin. Unhandled,
        // that error would take down the whole Theia backend, so it is caught here; 'exit' reports why.
        child.stdin.on('error', () => { /* reported by 'exit' */ });

        this.helloTimer = setTimeout(() => {
            if (this.child === child) {
                this.client?.onStatus('error', `The engine did not answer within ${HELLO_TIMEOUT_MS / 1000}s.`);
                this.stop();
            }
        }, HELLO_TIMEOUT_MS);
    }

    async stop(): Promise<void> {
        const child = this.child;
        if (!child) {
            return;
        }
        // From here on this process is on its way out; its 'exit' is expected, not an error (see onGone).
        this.child = undefined;
        clearTimeout(this.helloTimer);

        await new Promise<void>(resolve => {
            if (child.exitCode !== null || child.signalCode !== null) {
                resolve();   // already gone, 'exit' will not fire again
                return;
            }
            const kill = setTimeout(() => child.kill(), SHUTDOWN_GRACE_MS);
            child.once('exit', () => {
                clearTimeout(kill);
                resolve();
            });
            // Closing stdin is the engine's shutdown signal: it stops, flushes, and exits by itself.
            child.stdin.end();
        });
        this.client?.onStatus('offline');
    }

    dispose(): void {
        // Called when the frontend's connection closes: the client is a dead proxy by now.
        this.client = undefined;
        this.stop();
    }

    // ── commands: fire and forget ───────────────────────────────────────────────────────────

    async play(): Promise<void> {
        this.send({ type: EngineCommandType.Play });
    }

    async pause(): Promise<void> {
        this.send({ type: EngineCommandType.Stop });
    }

    async seek(beat: number): Promise<void> {
        this.send({ type: EngineCommandType.Seek, beat });
    }

    schema(enginePath: string): Promise<unknown> {
        return new Promise((resolve, reject) => {
            execFile(enginePath, ['schema'], { timeout: SCHEMA_TIMEOUT_MS, maxBuffer: 4 * 1024 * 1024 }, (error, stdout) => {
                if (error) {
                    reject(new Error(`Could not run ${enginePath} schema: ${error.message.split('\n')[0]}`));
                    return;
                }
                try {
                    resolve(JSON.parse(stdout));
                } catch {
                    reject(new Error(`${enginePath} schema did not print JSON.`));
                }
            });
        });
    }

    protected send(command: EngineCommand): void {
        if (!this.child) {
            throw new Error('The engine is not running.');
        }
        this.child.stdin.write(JSON.stringify(command) + '\n');
    }

    // ── what the engine says ────────────────────────────────────────────────────────────────

    protected onLine(child: ChildProcessWithoutNullStreams, line: string): void {
        if (child !== this.child) {
            return;   // a replaced engine's last words
        }
        let message: EngineMessage;
        try {
            message = JSON.parse(line);
        } catch {
            this.client?.onLog(`Unreadable line from the engine: ${line}`);
            return;
        }
        this.handle(message);
    }

    protected handle(message: EngineMessage): void {
        switch (message.type) {
            case EngineMessageType.Hello:
                clearTimeout(this.helloTimer);
                if (message.protocol_version !== PROTOCOL_VERSION) {
                    this.client?.onStatus('error', `The engine speaks protocol ${message.protocol_version}, `
                        + `the editor ${PROTOCOL_VERSION}. Update the one that is older.`);
                    this.stop();
                    return;
                }
                this.client?.onStatus('running');
                break;
            case EngineMessageType.Position:
                this.client?.onPosition({ beat: message.beat, playing: message.playing });
                break;
            case EngineMessageType.Error:
                this.client?.onLog(`Engine error: ${message.message}`);
                break;
            case EngineMessageType.Ok:
                break;   // fire and forget: nobody is waiting for it
        }
    }

    /** The process failed to start, or ended without `stop()`. Both can fire for one process. */
    protected onGone(child: ChildProcessWithoutNullStreams, reason: string): void {
        if (child !== this.child) {
            return;   // stopped on purpose (stop() cleared this.child first), or already reported
        }
        this.child = undefined;
        clearTimeout(this.helloTimer);
        this.client?.onStatus('error', reason);
    }
}