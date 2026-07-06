import * as vscode from 'vscode';
import { TransportSession } from '../engine/session';
import { webviewHtml, webviewOptions } from '../webview/html';
import type { ViewMsg } from '../protocol';

// The sidebar transport (Explorer section). Rendering is the Svelte
// `transport` bundle; this provider just bridges messages to the session.
export class TransportViewProvider implements vscode.WebviewViewProvider {
  static readonly viewType = 'codaw.transport';

  private view: vscode.WebviewView | undefined;

  constructor(
    private readonly extensionUri: vscode.Uri,
    private readonly session: TransportSession
  ) {
    session.subscribe((state) => {
      void this.view?.webview.postMessage({ type: 'transport', state });
    });
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view;
    view.webview.options = webviewOptions(this.extensionUri);
    view.webview.html = webviewHtml(view.webview, this.extensionUri, 'transport', 'CodaW Transport');

    view.webview.onDidReceiveMessage((msg: ViewMsg) => {
      switch (msg.type) {
        case 'ready':
          void view.webview.postMessage({ type: 'transport', state: this.session.state() });
          break;
        case 'transport':
          if (msg.action === 'toggle') {
            void this.session.togglePlay();
          } else if (msg.action === 'stop') {
            void this.session.stop();
          } else if (msg.action === 'seek' && msg.beat !== undefined) {
            void this.session.seek(msg.beat);
          }
          break;
      }
    });

    view.onDidDispose(() => {
      if (this.view === view) {
        this.view = undefined;
      }
    });
  }
}
