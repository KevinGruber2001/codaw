import * as vscode from 'vscode';
import { EngineClient } from './engineClient';

export interface TransportState {
  beat: number;
  playing: boolean;
  engineRunning: boolean;
}

// TransportSession owns the engine process and the session-global playback
// state. It is pure logic — no UI. Views (the sidebar transport, commands,
// future timeline panels) subscribe via onStateChange and call the verbs.
// Keeping the session separate from any one view means the engine survives
// the sidebar being collapsed/hidden: hiding UI must never kill the audio.
export class TransportSession {
  private client: EngineClient | undefined;
  private starting = false;

  beat = 0;
  playing = false;

  onStateChange?: (s: TransportState) => void;

  constructor(context: vscode.ExtensionContext) {
    context.subscriptions.push({ dispose: () => this.client?.dispose() });
  }

  state(): TransportState {
    return { beat: this.beat, playing: this.playing, engineRunning: this.client !== undefined };
  }

  async togglePlay(): Promise<void> {
    try {
      const client = await this.ensureEngine();
      if (this.playing) {
        await client.stop(); // engine Stop = pause-in-place
      } else {
        await client.play();
      }
    } catch (err) {
      void vscode.window.showErrorMessage(`CodaW: ${(err as Error).message}`);
    }
  }

  async stop(): Promise<void> {
    if (!this.client) {
      return; // nothing running — stop is a no-op, not an engine starter
    }
    try {
      await this.client.stop();
      await this.client.seek(0); // DAW convention: ⏹ returns to start
    } catch (err) {
      void vscode.window.showErrorMessage(`CodaW: ${(err as Error).message}`);
    }
  }

  async seek(beat: number): Promise<void> {
    if (!this.client) {
      return;
    }
    try {
      await this.client.seek(beat);
    } catch (err) {
      void vscode.window.showErrorMessage(`CodaW: ${(err as Error).message}`);
    }
  }

  private notify(): void {
    this.onStateChange?.(this.state());
  }

  // ensureEngine finds the project, spawns `codaw serve`, and wires events.
  // Lazy: the engine (and the audio device it claims) only exists once the
  // user actually asks for sound.
  private async ensureEngine(): Promise<EngineClient> {
    if (this.client) {
      return this.client;
    }
    if (this.starting) {
      throw new Error('engine is already starting');
    }
    this.starting = true;
    try {
      const projects = await vscode.workspace.findFiles('**/project.toml', '**/node_modules/**', 10);
      if (projects.length === 0) {
        throw new Error('no project.toml found in this workspace');
      }
      let project = projects[0];
      if (projects.length > 1) {
        const picked = await vscode.window.showQuickPick(
          projects.map((u) => ({ label: vscode.workspace.asRelativePath(u), uri: u })),
          { placeHolder: 'Multiple CodaW projects found — which one should the engine load?' }
        );
        if (!picked) {
          throw new Error('no project selected');
        }
        project = picked.uri;
      }

      // VS Code does not expand ${workspaceFolder} in extension settings —
      // that's a launch.json/tasks.json feature. We do it ourselves so the
      // repo can commit a portable .vscode/settings.json.
      let enginePath = vscode.workspace.getConfiguration('codaw').get<string>('enginePath', 'codaw');
      const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
      if (root) {
        enginePath = enginePath.replace('${workspaceFolder}', root);
      }
      const client = new EngineClient(enginePath, project.fsPath);

      client.onPosition = (pos) => {
        this.beat = pos.beat;
        this.playing = pos.playing;
        this.notify();
      };
      client.onError = (msg) => void vscode.window.showWarningMessage(`CodaW engine: ${msg}`);
      client.onExit = (code) => {
        this.client = undefined;
        this.playing = false;
        this.notify();
        if (code !== 0 && code !== null) {
          void vscode.window.showErrorMessage(
            `CodaW engine exited with code ${code} — is "${enginePath}" a codaw binary? Set codaw.enginePath in settings.`
          );
        }
      };

      try {
        await client.start();
      } catch (err) {
        client.dispose();
        throw new Error(
          `${(err as Error).message} — check the codaw.enginePath setting (currently "${enginePath}")`
        );
      }

      this.client = client;
      this.notify();
      return client;
    } finally {
      this.starting = false;
    }
  }
}
