import * as vscode from 'vscode';
import { TransportSession, TransportState } from './transport';

// TransportViewProvider renders the transport as a webview VIEW — a section
// inside the Explorer sidebar (see contributes.views in package.json), so it
// stays visible alongside the file tree whatever file is being edited.
//
// The view is a dumb terminal: it renders the state the session pushes and
// forwards button presses back. All real logic lives in TransportSession —
// collapsing or hiding the view must never affect playback.
export class TransportViewProvider implements vscode.WebviewViewProvider {
  static readonly viewType = 'codaw.transport';

  private view: vscode.WebviewView | undefined;

  constructor(private readonly session: TransportSession) {
    session.onStateChange = (s) => this.push(s);
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = { enableScripts: true };
    view.webview.html = this.html();

    view.webview.onDidReceiveMessage((msg) => {
      switch (msg.command) {
        case 'toggle':
          void this.session.togglePlay();
          break;
        case 'stop':
          void this.session.stop();
          break;
      }
    });

    view.onDidDispose(() => {
      if (this.view === view) {
        this.view = undefined;
      }
    });

    this.push(this.session.state());
  }

  private push(s: TransportState): void {
    void this.view?.webview.postMessage({ type: 'state', ...s });
  }

  // Styling leans entirely on VS Code's CSS variables so the transport looks
  // native in every theme, light or dark, without shipping any colors.
  private html(): string {
    return /* html */ `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
  body {
    font-family: var(--vscode-font-family);
    color: var(--vscode-foreground);
    padding: 8px 12px;
  }
  .row { display: flex; align-items: center; gap: 8px; }
  button {
    width: 34px; height: 28px;
    font-size: 14px; line-height: 1;
    color: var(--vscode-button-foreground);
    background: var(--vscode-button-background);
    border: none; border-radius: 3px;
    cursor: pointer;
  }
  button:hover { background: var(--vscode-button-hoverBackground); }
  #beat {
    margin-left: auto;
    font-family: var(--vscode-editor-font-family);
    font-size: 13px;
    font-variant-numeric: tabular-nums;
    color: var(--vscode-descriptionForeground);
  }
  #status {
    margin-top: 6px;
    font-size: 11px;
    color: var(--vscode-descriptionForeground);
  }
</style>
</head>
<body>
  <div class="row">
    <button id="play" title="Play / pause">&#9654;</button>
    <button id="stop" title="Stop and return to start">&#9632;</button>
    <span id="beat">0.0</span>
  </div>
  <div id="status">engine not running — press play</div>
  <script>
    const vscode = acquireVsCodeApi();
    const play = document.getElementById('play');
    const beat = document.getElementById('beat');
    const status = document.getElementById('status');
    play.addEventListener('click', () => vscode.postMessage({ command: 'toggle' }));
    document.getElementById('stop').addEventListener('click', () => vscode.postMessage({ command: 'stop' }));
    window.addEventListener('message', (e) => {
      const m = e.data;
      if (m.type !== 'state') return;
      play.innerHTML = m.playing ? '&#10074;&#10074;' : '&#9654;';
      beat.textContent = 'beat ' + m.beat.toFixed(1);
      status.textContent = m.engineRunning
        ? (m.playing ? 'playing' : 'paused')
        : 'engine not running — press play';
    });
  </script>
</body>
</html>`;
  }
}
