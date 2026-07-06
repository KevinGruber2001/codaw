import type { HostMsg, ViewMsg } from '../../src/protocol';

// Typed wrapper around the webview messaging API. acquireVsCodeApi() may be
// called exactly once per webview — this module is that one call site.

interface VsCodeApi {
  postMessage(msg: unknown): void;
}

declare function acquireVsCodeApi(): VsCodeApi;

const api = acquireVsCodeApi();

export function send(msg: ViewMsg): void {
  api.postMessage(msg);
}

export function onMessage(handler: (msg: HostMsg) => void): void {
  window.addEventListener('message', (e) => handler(e.data as HostMsg));
}

// Standard boot sequence: subscribe first, then announce readiness so the
// host knows it can push the initial state without racing the listener.
export function ready(handler: (msg: HostMsg) => void): void {
  onMessage(handler);
  send({ type: 'ready' });
}
