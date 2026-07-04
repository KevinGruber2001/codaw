import * as vscode from 'vscode';
import { parse as parseToml, stringify as stringifyToml } from 'smol-toml';

export class TrackEditorProvider implements vscode.CustomTextEditorProvider {
  constructor(private readonly context: vscode.ExtensionContext) {}

  async resolveCustomTextEditor(
    document: vscode.TextDocument,
    webviewPanel: vscode.WebviewPanel
  ) {
    webviewPanel.webview.options = { enableScripts: true };
    webviewPanel.webview.html = this.getHtml(webviewPanel.webview);

    const updateWebview = () => {
      const parsed = parseToml(document.getText());
      webviewPanel.webview.postMessage({ type: 'update', data: parsed });
    };

    // React to external file changes (e.g. your Go backend rewriting the file — live reload!)
    const changeSub = vscode.workspace.onDidChangeTextDocument(e => {
      if (e.document.uri.toString() === document.uri.toString()) {
        updateWebview();
      }
    });

    // React to messages FROM the webview (user edits a field in the UI)
    webviewPanel.webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'edit') {
        const newToml = stringifyToml(msg.data);
        const edit = new vscode.WorkspaceEdit();
        edit.replace(
          document.uri,
          new vscode.Range(0, 0, document.lineCount, 0),
          newToml
        );
        await vscode.workspace.applyEdit(edit);
      }
    });

    webviewPanel.onDidDispose(() => changeSub.dispose());

    updateWebview(); // initial load
  }

  private getHtml(webview: vscode.Webview): string {
    return /* html */ `
      <!DOCTYPE html>
      <html>
        <body>
          <div id="app"></div>
          <script>
            const vscode = acquireVsCodeApi();
            window.addEventListener('message', e => {
              if (e.data.type === 'update') {
                // render form based on e.data.data
              }
            });
            function sendEdit(newData) {
              vscode.postMessage({ type: 'edit', data: newData });
            }
          </script>
        </body>
      </html>
    `;
  }
}