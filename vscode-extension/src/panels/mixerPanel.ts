import * as vscode from 'vscode';
import * as path from 'path';
import { parse as parseToml } from 'smol-toml';
import { webviewHtml, webviewOptions } from '../webview/html';
import { applyScalarEdit } from '../toml/edits';
import type { MixerChannel, ViewMsg } from '../protocol';

type Toml = Record<string, unknown>;

// MixerPanel is the first cross-file view: one editor-area panel showing
// every track, bus, and master as channel strips. Unlike the per-file
// editors it aggregates MANY documents — the project's layout decides which
// files belong to the session, and each strip edit routes back to its own
// file. Opened via the command palette (codaw.openMixer); a singleton, so
// re-running the command reveals the existing panel.
export class MixerPanel {
  private static current: MixerPanel | undefined;

  private readonly panel: vscode.WebviewPanel;
  private readonly watcher: vscode.Disposable;

  static open(extensionUri: vscode.Uri): void {
    if (MixerPanel.current) {
      MixerPanel.current.panel.reveal();
      return;
    }
    MixerPanel.current = new MixerPanel(extensionUri);
  }

  private constructor(extensionUri: vscode.Uri) {
    this.panel = vscode.window.createWebviewPanel(
      'codaw.mixer',
      'CodaW Mixer',
      vscode.ViewColumn.Active,
      { ...webviewOptions(extensionUri), retainContextWhenHidden: true }
    );
    this.panel.webview.html = webviewHtml(this.panel.webview, extensionUri, 'mixer', 'CodaW Mixer');

    this.panel.webview.onDidReceiveMessage(async (msg: ViewMsg) => {
      if (msg.type === 'ready') {
        await this.push();
      } else if (msg.type === 'edit' && msg.file) {
        const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(msg.file));
        if (!(await applyScalarEdit(doc, msg.key, msg.value))) {
          void vscode.window.showWarningMessage(`codaw: could not edit ${msg.key} in ${msg.file}`);
        }
        // The save triggers onDidSaveTextDocument below, which re-pushes.
      }
    });

    // The mixer mirrors files on disk: any saved TOML in the workspace may
    // change the session (values, or the layout itself), so re-read on save.
    this.watcher = vscode.workspace.onDidSaveTextDocument((doc) => {
      if (doc.fileName.endsWith('.toml')) {
        void this.push();
      }
    });

    this.panel.onDidDispose(() => {
      this.watcher.dispose();
      MixerPanel.current = undefined;
    });
  }

  private async push(): Promise<void> {
    const channels = await gatherChannels();
    void this.panel.webview.postMessage({ type: 'mixer', channels });
  }
}

// gatherChannels loads the session exactly the way the Go loader does:
// project.toml's [layout] section names the files, resolved relative to the
// project root. That keeps the mixer's world view identical to the engine's.
async function gatherChannels(): Promise<MixerChannel[]> {
  const projects = await vscode.workspace.findFiles('**/project.toml', '**/node_modules/**', 1);
  if (projects.length === 0) {
    return [];
  }
  const projectUri = projects[0];
  const rootDir = path.dirname(projectUri.fsPath);

  const project = await readToml(projectUri.fsPath);
  if (!project) {
    return [];
  }
  const layout = (project.layout ?? {}) as Toml;
  const trackFiles = Array.isArray(layout.tracks) ? (layout.tracks as string[]) : [];
  const busFiles = Array.isArray(layout.buses) ? (layout.buses as string[]) : [];
  const masterFile = typeof layout.master === 'string' ? layout.master : 'master.toml';

  const channels: MixerChannel[] = [];

  for (const rel of trackFiles) {
    const file = path.join(rootDir, rel);
    const raw = await readToml(file);
    if (raw) {
      channels.push({
        file,
        kind: 'track',
        id: String(raw.id ?? path.basename(rel, '.toml')),
        gain: Number(raw.gain ?? 0),
        pan: Number(raw.pan ?? 0),
        mute: Boolean(raw.mute ?? false),
        solo: Boolean(raw.solo ?? false),
        bus: String(raw.bus ?? ''),
      });
    }
  }

  for (const rel of busFiles) {
    const file = path.join(rootDir, rel);
    const raw = await readToml(file);
    if (raw) {
      channels.push({
        file,
        kind: 'bus',
        id: String(raw.id ?? path.basename(rel, '.toml')),
        gain: Number(raw.gain ?? 0),
      });
    }
  }

  const masterPath = path.join(rootDir, masterFile);
  const master = await readToml(masterPath);
  if (master) {
    channels.push({
      file: masterPath,
      kind: 'master',
      id: 'master',
      gain: Number(master.gain ?? 0),
    });
  }

  return channels;
}

async function readToml(file: string): Promise<Toml | undefined> {
  try {
    const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(file));
    return parseToml(doc.getText()) as Toml;
  } catch {
    return undefined; // missing or invalid file: skip the channel, not the mixer
  }
}
