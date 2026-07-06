import * as vscode from 'vscode';
import * as path from 'path';
import { webviewHtml, webviewOptions } from '../webview/html';
import { applyScalarEdit } from '../toml/edits';
import { readSessionLayout, readToml } from '../toml/sessionReader';
import type { MixerChannel, ViewMsg } from '../protocol';

// MixerPanel is a cross-file view: one editor-area panel showing every track,
// bus, and master as channel strips. Unlike the per-file editors it
// aggregates MANY documents — the project's layout decides which files belong
// to the session, and each strip edit routes back to its own file. Opened via
// the command palette (codaw.openMixer); a singleton, so re-running the
// command reveals the existing panel.
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

// gatherChannels loads the session via the shared layout reader, so the
// mixer's world view is identical to the engine's (and the arrangement's).
async function gatherChannels(): Promise<MixerChannel[]> {
  const layout = await readSessionLayout();
  if (!layout) {
    return [];
  }

  const channels: MixerChannel[] = [];

  for (const file of layout.trackFiles) {
    const raw = await readToml(file);
    if (raw) {
      channels.push({
        file,
        kind: 'track',
        id: String(raw.id ?? path.basename(file, '.toml')),
        gain: Number(raw.gain ?? 0),
        pan: Number(raw.pan ?? 0),
        mute: Boolean(raw.mute ?? false),
        solo: Boolean(raw.solo ?? false),
        bus: String(raw.bus ?? ''),
      });
    }
  }

  for (const file of layout.busFiles) {
    const raw = await readToml(file);
    if (raw) {
      channels.push({
        file,
        kind: 'bus',
        id: String(raw.id ?? path.basename(file, '.toml')),
        gain: Number(raw.gain ?? 0),
      });
    }
  }

  const master = await readToml(layout.masterFile);
  if (master) {
    channels.push({
      file: layout.masterFile,
      kind: 'master',
      id: 'master',
      gain: Number(master.gain ?? 0),
    });
  }

  return channels;
}
