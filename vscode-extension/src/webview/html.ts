import * as vscode from 'vscode';

// Shared HTML scaffold for every CodaW webview. Each view is a Vite-built
// Svelte entry: media/<entry>.js (+ shared media/style.css). Assets must be
// referenced through asWebviewUri — webviews can't load file:// paths.
//
// CSP note: scripts are allowed from the extension's webview origin
// (cspSource) rather than via nonce. Vite emits ES modules that import shared
// chunks, and `import` statements can't carry a nonce — origin-scoping is the
// standard answer. 'unsafe-inline' styles are needed for Svelte's inline
// style attributes (style-src blocks the attribute form without it).
export function webviewHtml(
  webview: vscode.Webview,
  extensionUri: vscode.Uri,
  entry: string,
  title: string
): string {
  const js = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'media', `${entry}.js`));
  const css = webview.asWebviewUri(vscode.Uri.joinPath(extensionUri, 'media', 'style.css'));

  return /* html */ `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta http-equiv="Content-Security-Policy"
        content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src ${webview.cspSource}; font-src ${webview.cspSource}; img-src ${webview.cspSource};">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>${title}</title>
  <link rel="stylesheet" href="${css}">
</head>
<body>
  <div id="app"></div>
  <script type="module" src="${js}"></script>
</body>
</html>`;
}

// Standard webview options: scripts on, assets restricted to media/.
export function webviewOptions(extensionUri: vscode.Uri): vscode.WebviewOptions {
  return {
    enableScripts: true,
    localResourceRoots: [vscode.Uri.joinPath(extensionUri, 'media')],
  };
}
