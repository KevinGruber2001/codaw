import * as vscode from 'vscode';
import { parse as parseToml } from 'smol-toml';

// TrackEditorProvider is the custom editor bound to tracks/*.toml.
//
// Data-flow contract (important for a code-first tool):
//
//   READ  — the whole document is parsed and pushed to the webview as data.
//   WRITE — the webview asks to change ONE value ({ key, value }) and we make
//           a minimal text edit to just that value's range in the document.
//
// We deliberately never regenerate the file from the parsed object
// (parse → stringify): that would destroy comments, key order, and formatting
// — the user's hand-written TOML is the product here, and a UI tweak must
// produce the same one-line git diff a hand edit would.
export class TrackEditorProvider implements vscode.CustomTextEditorProvider {
  constructor(private readonly context: vscode.ExtensionContext) {}

  async resolveCustomTextEditor(
    document: vscode.TextDocument,
    webviewPanel: vscode.WebviewPanel
  ) {
    webviewPanel.webview.options = { enableScripts: true };
    webviewPanel.webview.html = this.getHtml();

    // Version of the document produced by OUR most recent edit. Used to break
    // the echo loop: our applyEdit fires onDidChangeTextDocument like any
    // other edit, but the webview already knows that value — re-sending it
    // would re-render controls mid-drag.
    let selfEditVersion = -1;

    const updateWebview = () => {
      let parsed: unknown;
      try {
        parsed = parseToml(document.getText());
      } catch {
        // Mid-edit TOML is often momentarily invalid — keep the last good
        // state in the webview instead of erroring.
        return;
      }
      webviewPanel.webview.postMessage({ type: 'update', data: parsed });
    };

    const changeSub = vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.uri.toString() !== document.uri.toString()) {
        return;
      }
      if (e.document.version === selfEditVersion) {
        return; // our own edit echoing back — webview is already current
      }
      updateWebview();
    });

    webviewPanel.webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'edit') {
        const edit = buildScalarEdit(document, msg.key, msg.value);
        if (!edit) {
          void vscode.window.showWarningMessage(
            `codaw: could not find top-level key "${msg.key}" in ${document.fileName}`
          );
          return;
        }
        const we = new vscode.WorkspaceEdit();
        we.replace(document.uri, edit.range, edit.newText);
        const applied = await vscode.workspace.applyEdit(we);
        if (applied) {
          selfEditVersion = document.version;
        }
      }
    });

    webviewPanel.onDidDispose(() => changeSub.dispose());

    updateWebview();
  }

  private getHtml(): string {
    return /* html */ `
      <!DOCTYPE html>
      <html>
        <body>
          <div id="app"></div>
          <script>
            const vscode = acquireVsCodeApi();
            window.addEventListener('message', e => {
              if (e.data.type === 'update') {
                // render controls based on e.data.data
              }
            });
            function sendEdit(key, value) {
              vscode.postMessage({ type: 'edit', key, value });
            }
          </script>
        </body>
      </html>
    `;
  }
}

// buildScalarEdit locates a top-level `key = value` line and returns a range
// covering ONLY the value text (comments and whitespace untouched).
//
// "Top-level" means before the first table header ([section] / [[array]]) —
// which for track files covers id, bus, gain, pan, mute, solo. Section-scoped
// values (fx params, clip fields) need position-aware parsing and come later.
export function buildScalarEdit(
  document: vscode.TextDocument,
  key: string,
  value: number | boolean | string
): { range: vscode.Range; newText: string } | undefined {
  const keyRe = new RegExp(`^\\s*${escapeRegExp(key)}\\s*=\\s*`);

  for (let line = 0; line < document.lineCount; line++) {
    const text = document.lineAt(line).text;

    // Stop at the first table header — past it we're no longer top-level.
    if (/^\s*\[/.test(text)) {
      break;
    }

    const m = keyRe.exec(text);
    if (!m) {
      continue;
    }

    // Value runs from the end of "key = " to the start of an inline comment
    // (or end of line), minus trailing spaces we want to leave in place.
    const valueStart = m[0].length;
    const hash = text.indexOf('#', valueStart);
    const rawEnd = hash >= 0 ? hash : text.length;
    let valueEnd = rawEnd;
    while (valueEnd > valueStart && text[valueEnd - 1] === ' ') {
      valueEnd--;
    }

    return {
      range: new vscode.Range(line, valueStart, line, valueEnd),
      newText: formatTomlValue(value),
    };
  }
  return undefined;
}

function formatTomlValue(v: number | boolean | string): string {
  if (typeof v === 'string') {
    return JSON.stringify(v); // TOML basic strings share JSON's escaping
  }
  if (typeof v === 'number' && Number.isFinite(v) && Number.isInteger(v)) {
    // codaw's numeric fields are floats (gain, pan) — keep a decimal point so
    // the TOML type stays float and diffs stay stylistically consistent.
    return v.toFixed(1);
  }
  return String(v);
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
