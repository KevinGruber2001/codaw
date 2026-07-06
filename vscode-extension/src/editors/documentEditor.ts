import * as vscode from 'vscode';
import { parse as parseToml } from 'smol-toml';
import { webviewHtml, webviewOptions } from '../webview/html';
import { applyScalarEdit, Target } from '../toml/edits';
import type { ViewMsg } from '../protocol';

// One generic CustomTextEditorProvider drives every per-file editor. A view
// kind is pure configuration: which files it binds to (package.json), which
// Svelte bundle renders it, how raw TOML maps to the webview's data shape,
// and where its editable keys live in the file. Adding an editor = adding an
// EditorKind — no new provider code.

type Toml = Record<string, unknown>;

export interface EditorKind {
  viewType: string;
  /** Svelte entry name → media/<entry>.js */
  entry: string;
  title: string;
  /** Raw parsed TOML → the protocol data shape the view expects. */
  map(raw: Toml): unknown;
  /** Where a top-level UI key lives in the TOML (e.g. project "bpm" → [transport]). */
  keyTarget?(key: string): Target;
}

// ── TOML → protocol mappers ────────────────────────────────────────────────
// smol-toml returns plain objects mirroring the file; these normalise them
// (defaults for omitted keys, fx params split from their `type`).

function mapFx(raw: unknown): { type: string; params: Record<string, number> }[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.map((f) => {
    const { type, ...rest } = f as Toml & { type?: string };
    const params: Record<string, number> = {};
    for (const [k, v] of Object.entries(rest)) {
      if (typeof v === 'number') {
        params[k] = v;
      }
    }
    return { type: String(type ?? 'unknown'), params };
  });
}

export const trackKind: EditorKind = {
  viewType: 'codaw.trackEditor',
  entry: 'track',
  title: 'CodaW Track',
  map(raw) {
    return {
      id: raw.id ?? '',
      bus: raw.bus ?? '',
      gain: raw.gain ?? 0,
      pan: raw.pan ?? 0,
      mute: raw.mute ?? false,
      solo: raw.solo ?? false,
      fx: mapFx(raw.fx),
      clips: (Array.isArray(raw.clip) ? raw.clip : []).map((c) => {
        const clip = c as Toml;
        return {
          file: clip.file ?? '',
          start: clip.start ?? 0,
          end: clip.end ?? 0,
          offset: clip.offset ?? 0,
          loop: clip.loop ?? false,
          gain: clip.gain ?? 0,
        };
      }),
      automation: (Array.isArray(raw.automation) ? raw.automation : []).map((a) => {
        const lane = a as Toml;
        return {
          target: lane.target ?? '',
          points: (Array.isArray(lane.point) ? lane.point : []).map((p) => {
            const pt = p as Toml;
            return { beat: pt.beat ?? 0, value: pt.value ?? 0 };
          }),
        };
      }),
    };
  },
};

export const busKind: EditorKind = {
  viewType: 'codaw.busEditor',
  entry: 'bus',
  title: 'CodaW Bus',
  map(raw) {
    return { id: raw.id ?? '', gain: raw.gain ?? 0, fx: mapFx(raw.fx) };
  },
};

export const masterKind: EditorKind = {
  viewType: 'codaw.masterEditor',
  entry: 'master',
  title: 'CodaW Master',
  map(raw) {
    return { gain: raw.gain ?? 0, limiter: raw.limiter ?? false, fx: mapFx(raw.fx) };
  },
};

export const projectKind: EditorKind = {
  viewType: 'codaw.projectEditor',
  entry: 'project',
  title: 'CodaW Project',
  map(raw) {
    const meta = (raw.project ?? {}) as Toml;
    const transport = (raw.transport ?? {}) as Toml;
    const layout = (raw.layout ?? {}) as Toml;
    return {
      name: meta.name ?? 'untitled',
      bpm: transport.bpm ?? 120,
      timeSig: transport.time_sig ?? '4/4',
      sampleRate: transport.sample_rate ?? 48000,
      bitDepth: transport.bit_depth ?? 32,
      tracks: Array.isArray(layout.tracks) ? layout.tracks : [],
      buses: Array.isArray(layout.buses) ? layout.buses : [],
    };
  },
  // project.toml keys live under section headers, not at top level.
  keyTarget(key) {
    if (key === 'bpm' || key === 'time_sig') {
      return { section: 'transport' };
    }
    if (key === 'name') {
      return { section: 'project' };
    }
    return {};
  },
};

// ── The provider ───────────────────────────────────────────────────────────

export class DocumentEditorProvider implements vscode.CustomTextEditorProvider {
  constructor(
    private readonly extensionUri: vscode.Uri,
    private readonly kind: EditorKind
  ) {}

  static register(context: vscode.ExtensionContext, kind: EditorKind): vscode.Disposable {
    return vscode.window.registerCustomEditorProvider(
      kind.viewType,
      new DocumentEditorProvider(context.extensionUri, kind),
      { webviewOptions: { retainContextWhenHidden: true } }
    );
  }

  async resolveCustomTextEditor(
    document: vscode.TextDocument,
    panel: vscode.WebviewPanel
  ): Promise<void> {
    panel.webview.options = webviewOptions(this.extensionUri);
    panel.webview.html = webviewHtml(panel.webview, this.extensionUri, this.kind.entry, this.kind.title);

    // Version of the document produced by OUR most recent edit — breaks the
    // echo loop (our applyEdit fires onDidChangeTextDocument like any edit,
    // but the webview already shows that value; re-pushing would fight an
    // in-progress drag).
    let selfEditVersion = -1;

    const push = () => {
      try {
        const raw = parseToml(document.getText()) as Toml;
        void panel.webview.postMessage({ type: 'update', data: this.kind.map(raw) });
      } catch (err) {
        // Mid-edit TOML is often momentarily invalid; tell the view instead
        // of pushing broken state.
        void panel.webview.postMessage({ type: 'invalid', message: (err as Error).message });
      }
    };

    const changeSub = vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.uri.toString() !== document.uri.toString()) {
        return;
      }
      if (e.document.version === selfEditVersion) {
        return;
      }
      push();
    });

    panel.webview.onDidReceiveMessage(async (msg: ViewMsg) => {
      switch (msg.type) {
        case 'ready':
          push();
          break;
        case 'edit': {
          const target = this.kind.keyTarget?.(msg.key) ?? {};
          if (await applyScalarEdit(document, msg.key, msg.value, target)) {
            selfEditVersion = document.version;
          } else {
            void vscode.window.showWarningMessage(
              `codaw: could not find key "${msg.key}" in ${document.fileName}`
            );
          }
          break;
        }
        case 'fxEdit': {
          const ok = await applyScalarEdit(document, msg.key, msg.value, {
            section: 'fx',
            index: msg.index,
          });
          if (ok) {
            selfEditVersion = document.version;
          } else {
            void vscode.window.showWarningMessage(
              `codaw: could not find fx[${msg.index}].${msg.key} in ${document.fileName}`
            );
          }
          break;
        }
      }
    });

    panel.onDidDispose(() => changeSub.dispose());
  }
}
