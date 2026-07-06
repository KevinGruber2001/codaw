import * as vscode from 'vscode';
import { webviewHtml, webviewOptions } from '../webview/html';
import { applyScalarEdit, applyScalarEdits } from '../toml/edits';
import { readSessionLayout, readToml } from '../toml/sessionReader';
import { TransportSession } from '../engine/session';
import type { ArrangementData, ArrangementTrack, ViewMsg } from '../protocol';

// ArrangementPanel is the "main screen": every track as a horizontal lane,
// clips placed in time, a live playhead. Like the mixer it aggregates the
// whole session (layout-driven); unlike the mixer it also talks to the
// transport — the playhead streams in, ruler clicks seek.
//
// Clip drags write [[clip]] start/end back to the track's TOML atomically;
// the engine's watcher treats timing changes as structural and rebuilds,
// resuming at the playhead.
export class ArrangementPanel {
  private static current: ArrangementPanel | undefined;

  private readonly panel: vscode.WebviewPanel;
  private readonly disposables: vscode.Disposable[] = [];

  static open(extensionUri: vscode.Uri, session: TransportSession): void {
    if (ArrangementPanel.current) {
      ArrangementPanel.current.panel.reveal();
      return;
    }
    ArrangementPanel.current = new ArrangementPanel(extensionUri, session);
  }

  private constructor(extensionUri: vscode.Uri, session: TransportSession) {
    this.panel = vscode.window.createWebviewPanel(
      'codaw.arrangement',
      'CodaW Arrangement',
      vscode.ViewColumn.Active,
      { ...webviewOptions(extensionUri), retainContextWhenHidden: true }
    );
    this.panel.webview.html = webviewHtml(this.panel.webview, extensionUri, 'arrangement', 'CodaW Arrangement');

    this.panel.webview.onDidReceiveMessage(async (msg: ViewMsg) => {
      switch (msg.type) {
        case 'ready':
          await this.push();
          void this.panel.webview.postMessage({ type: 'transport', state: session.state() });
          break;
        case 'edit': {
          if (!msg.file) {
            return;
          }
          const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(msg.file));
          if (!(await applyScalarEdit(doc, msg.key, msg.value))) {
            void vscode.window.showWarningMessage(`codaw: could not edit ${msg.key} in ${msg.file}`);
          }
          break;
        }
        case 'clipMove': {
          const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(msg.file));
          const ok = await applyScalarEdits(doc, [
            { key: 'start', value: msg.start, target: { section: 'clip', index: msg.index } },
            { key: 'end', value: msg.end, target: { section: 'clip', index: msg.index } },
          ]);
          if (!ok) {
            void vscode.window.showWarningMessage(`codaw: could not move clip[${msg.index}] in ${msg.file}`);
          }
          break;
        }
        case 'transport':
          if (msg.action === 'toggle') {
            void session.togglePlay();
          } else if (msg.action === 'stop') {
            void session.stop();
          } else if (msg.action === 'seek' && msg.beat !== undefined) {
            void session.seek(msg.beat);
          }
          break;
      }
    });

    // Live playhead: forward every session state change.
    this.disposables.push(
      session.subscribe((state) => {
        void this.panel.webview.postMessage({ type: 'transport', state });
      }),
      // The arrangement mirrors disk state — re-read on any TOML save.
      vscode.workspace.onDidSaveTextDocument((doc) => {
        if (doc.fileName.endsWith('.toml')) {
          void this.push();
        }
      })
    );

    this.panel.onDidDispose(() => {
      for (const d of this.disposables) {
        d.dispose();
      }
      ArrangementPanel.current = undefined;
    });
  }

  private async push(): Promise<void> {
    const data = await gatherArrangement();
    if (data) {
      void this.panel.webview.postMessage({ type: 'arrangement', data });
    }
  }
}

async function gatherArrangement(): Promise<ArrangementData | undefined> {
  const layout = await readSessionLayout();
  if (!layout) {
    return undefined;
  }

  const tracks: ArrangementTrack[] = [];
  for (const file of layout.trackFiles) {
    const raw = await readToml(file);
    if (!raw) {
      continue;
    }
    const clips = Array.isArray(raw.clip) ? raw.clip : [];
    tracks.push({
      file,
      id: String(raw.id ?? ''),
      mute: Boolean(raw.mute ?? false),
      solo: Boolean(raw.solo ?? false),
      clips: clips.map((c) => {
        const clip = c as Record<string, unknown>;
        return {
          file: String(clip.file ?? ''),
          start: Number(clip.start ?? 0),
          end: Number(clip.end ?? 0),
          offset: Number(clip.offset ?? 0),
          loop: Boolean(clip.loop ?? false),
          gain: Number(clip.gain ?? 0),
        };
      }),
    });
  }

  return { bpm: layout.bpm, beatsPerBar: layout.beatsPerBar, tracks };
}
