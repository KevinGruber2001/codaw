// Message types exchanged between the extension host and its webviews.
// ONE file, imported by both sides (the host via webpack, the Svelte apps via
// Vite), so the two ends of postMessage can never drift apart silently.
//
// Naming convention: `HostMsg` flows host → webview, `ViewMsg` flows
// webview → host. Every message has a `type` discriminant.

// ── Data shapes (mirror the TOML schema; see codaw/internal/project) ────────

export interface FxData {
  type: string;
  params: Record<string, number>;
}

export interface ClipData {
  file: string;
  start: number;
  end: number;
  offset: number;
  loop: boolean;
  gain: number;
}

export interface AutomationData {
  target: string;
  points: { beat: number; value: number }[];
}

export interface TrackData {
  id: string;
  bus: string;
  gain: number;
  pan: number;
  mute: boolean;
  solo: boolean;
  fx: FxData[];
  clips: ClipData[];
  automation: AutomationData[];
}

export interface BusData {
  id: string;
  gain: number;
  fx: FxData[];
}

export interface MasterData {
  gain: number;
  limiter: boolean;
  fx: FxData[];
}

export interface ProjectData {
  name: string;
  bpm: number;
  timeSig: string;
  sampleRate: number;
  bitDepth: number;
  tracks: string[]; // layout order (file paths)
  buses: string[];
}

// The mixer needs the whole session at once. `file` lets edits route back to
// the right document.
export interface MixerChannel {
  file: string;
  kind: 'track' | 'bus' | 'master';
  id: string; // "master" for the master channel
  gain: number;
  pan?: number; // tracks only
  mute?: boolean;
  solo?: boolean;
  bus?: string; // routing label, tracks only
}

export interface TransportState {
  beat: number;
  playing: boolean;
  engineRunning: boolean;
}

// ── Host → webview ──────────────────────────────────────────────────────────

export type HostMsg =
  | { type: 'update'; data: unknown } // per-file editors: freshly parsed TOML
  | { type: 'mixer'; channels: MixerChannel[] }
  | { type: 'transport'; state: TransportState }
  | { type: 'invalid'; message: string }; // document currently unparseable

// ── Webview → host ──────────────────────────────────────────────────────────

export type ViewMsg =
  // Webview finished mounting; host replies with the current state. Without
  // this handshake, a postMessage sent before the listener exists is lost.
  | { type: 'ready' }
  // Set one top-level scalar in a TOML document. `file` is only set by
  // cross-file views (mixer); per-file editors imply their own document.
  | { type: 'edit'; file?: string; key: string; value: number | boolean | string }
  // Set one parameter of the Nth [[fx]] block.
  | { type: 'fxEdit'; file?: string; index: number; key: string; value: number }
  // Transport verbs (any view may host transport controls).
  | { type: 'transport'; action: 'toggle' | 'stop' | 'seek'; beat?: number };
