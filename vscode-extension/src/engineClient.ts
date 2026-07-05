import { spawn, ChildProcessWithoutNullStreams } from 'child_process';
import * as readline from 'readline';

// EngineClient owns a `codaw serve` child process and speaks CodaW's control
// protocol: newline-delimited JSON on the child's stdin/stdout (see
// docs/ipc.md in the repo). The lifecycle rule is structural: the engine is
// our child, so it can never outlive the extension — closing stdin is the
// shutdown signal.
//
// Split of responsibilities (mirrors the engine side):
//   TOML files  — project data. The client NEVER sends project edits here;
//                 the editor writes files and the engine's watcher reloads.
//   this channel — ephemeral session data only: play/stop/seek in,
//                 position/state events out.

/** The protocol revision this client understands (must match serve.ProtocolVersion). */
const PROTOCOL_VERSION = 1;

export interface Position {
  beat: number;
  playing: boolean;
}

interface Pending {
  resolve: () => void;
  reject: (err: Error) => void;
}

export class EngineClient {
  private child: ChildProcessWithoutNullStreams | undefined;
  private nextId = 1;
  private pending = new Map<number, Pending>();

  /** Fired ~10x/sec while playing, and once on every play/stop/seek. */
  onPosition?: (pos: Position) => void;
  /** Engine-side errors that aren't tied to a specific request. */
  onError?: (message: string) => void;
  /** The engine process exited (crash or normal shutdown). */
  onExit?: (code: number | null) => void;

  constructor(
    private readonly binaryPath: string,
    private readonly projectToml: string
  ) {}

  /** Spawns the engine and resolves once a compatible hello arrives. */
  start(): Promise<void> {
    return new Promise((resolve, reject) => {
      const child = spawn(this.binaryPath, ['serve', this.projectToml]);
      this.child = child;

      let helloSeen = false;
      const helloTimeout = setTimeout(() => {
        if (!helloSeen) {
          this.dispose();
          reject(new Error('engine did not say hello within 10s'));
        }
      }, 10_000);

      readline.createInterface({ input: child.stdout }).on('line', (line) => {
        let msg: any;
        try {
          msg = JSON.parse(line);
        } catch {
          return; // tolerate garbage rather than dying — protocol rule
        }

        if (!helloSeen) {
          if (msg.type !== 'hello') {
            return;
          }
          helloSeen = true;
          clearTimeout(helloTimeout);
          if (msg.protocol_version !== PROTOCOL_VERSION) {
            this.dispose();
            reject(new Error(
              `engine speaks protocol v${msg.protocol_version}, this extension needs v${PROTOCOL_VERSION} — update codaw or the extension`
            ));
            return;
          }
          resolve();
          return;
        }

        this.handleMessage(msg);
      });

      // stderr = engine logs. Surface them for debugging; never parse them.
      child.stderr.on('data', (d: Buffer) => {
        console.log(`[codaw engine] ${d.toString().trimEnd()}`);
      });

      child.on('exit', (code) => {
        // Fail anything still awaiting a reply — the replies will never come.
        for (const p of this.pending.values()) {
          p.reject(new Error('engine exited'));
        }
        this.pending.clear();
        this.child = undefined;
        this.onExit?.(code);
      });

      child.on('error', (err) => {
        clearTimeout(helloTimeout);
        reject(new Error(`failed to spawn codaw: ${err.message}`));
      });
    });
  }

  play(): Promise<void> {
    return this.request({ type: 'play' });
  }

  stop(): Promise<void> {
    return this.request({ type: 'stop' });
  }

  seek(beat: number): Promise<void> {
    return this.request({ type: 'seek', beat });
  }

  /**
   * Shuts the engine down by closing its stdin (EOF is the protocol's
   * shutdown signal). If it hasn't exited shortly after, kill it — a wedged
   * engine holding the audio device is worse than an ungraceful exit.
   */
  dispose(): void {
    const child = this.child;
    if (!child) {
      return;
    }
    child.stdin.end();
    const killer = setTimeout(() => child.kill(), 3_000);
    child.on('exit', () => clearTimeout(killer));
  }

  private handleMessage(msg: any): void {
    // Replies carry the id we sent; events don't.
    if (typeof msg.id === 'number' && this.pending.has(msg.id)) {
      const p = this.pending.get(msg.id)!;
      this.pending.delete(msg.id);
      if (msg.type === 'error') {
        p.reject(new Error(msg.message ?? 'engine error'));
      } else {
        p.resolve();
      }
      return;
    }

    switch (msg.type) {
      case 'position':
        this.onPosition?.({ beat: msg.beat, playing: msg.playing });
        break;
      case 'error':
        this.onError?.(msg.message ?? 'unknown engine error');
        break;
      default:
        // Unknown event types are fine — forward compatibility by design.
        break;
    }
  }

  private request(msg: { type: string; beat?: number }): Promise<void> {
    const child = this.child;
    if (!child) {
      return Promise.reject(new Error('engine not running'));
    }
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      child.stdin.write(JSON.stringify({ id, ...msg }) + '\n');
    });
  }
}
