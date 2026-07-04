import * as vscode from 'vscode';

export function createWebviewPanel(context: vscode.ExtensionContext) {
  const panel = vscode.window.createWebviewPanel(
    'myWebview', 'My Webview', vscode.ViewColumn.One, { enableScripts: true }
  );

  panel.webview.html = getWebviewContent();
}

function getWebviewContent(): string {
  return `
    <!DOCTYPE html>
    <html lang="en">
    <head><meta charset="UTF-8" /><title>Webview</title></head>
    <body>
      <h1>Hello from Webview!</h1>
      <button onclick="sendMessage()">Click me</button>
      <script>
        const vscode = acquireVsCodeApi();
        function sendMessage() {
          vscode.postMessage({ command: 'alert', text: 'Button clicked!' });
        }
      </script>
    </body>
    </html>
  `;
}