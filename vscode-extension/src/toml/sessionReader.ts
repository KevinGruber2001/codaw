import * as vscode from 'vscode';
import * as path from 'path';
import { parse as parseToml } from 'smol-toml';

export type Toml = Record<string, unknown>;

// Reads the session the same way the Go loader does: project.toml's [layout]
// names the member files, resolved relative to the project's directory.
// Keeping this in one place guarantees every cross-file view (mixer,
// arrangement, future timeline tools) sees the identical session the engine
// plays.

export interface SessionLayout {
  rootDir: string;
  projectFile: string;
  trackFiles: string[]; // absolute, in layout order (= UI display order)
  busFiles: string[];
  masterFile: string;
  bpm: number;
  beatsPerBar: number;
}

export async function readSessionLayout(): Promise<SessionLayout | undefined> {
  const projects = await vscode.workspace.findFiles('**/project.toml', '**/node_modules/**', 1);
  if (projects.length === 0) {
    return undefined;
  }
  const projectFile = projects[0].fsPath;
  const rootDir = path.dirname(projectFile);

  const raw = await readToml(projectFile);
  if (!raw) {
    return undefined;
  }
  const layout = (raw.layout ?? {}) as Toml;
  const transport = (raw.transport ?? {}) as Toml;

  const rel = (v: unknown): string[] =>
    Array.isArray(v) ? (v as string[]).map((f) => path.join(rootDir, f)) : [];

  // time_sig "4/4" → 4 beats per bar (the numerator is all the ruler needs).
  const timeSig = String(transport.time_sig ?? '4/4');
  const beatsPerBar = Number(timeSig.split('/')[0]) || 4;

  return {
    rootDir,
    projectFile,
    trackFiles: rel(layout.tracks),
    busFiles: rel(layout.buses),
    masterFile: path.join(rootDir, String(layout.master ?? 'master.toml')),
    bpm: Number(transport.bpm ?? 120),
    beatsPerBar,
  };
}

export async function readToml(file: string): Promise<Toml | undefined> {
  try {
    const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(file));
    return parseToml(doc.getText()) as Toml;
  } catch {
    return undefined; // missing or momentarily invalid — caller skips it
  }
}
