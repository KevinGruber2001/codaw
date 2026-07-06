import * as vscode from 'vscode';

// Targeted TOML edits. The core rule of the whole extension lives here:
// UI edits NEVER regenerate a file (parse → stringify destroys comments, key
// order, and formatting). Instead we locate the one `key = value` line and
// replace only the value's character range, so a fader drag produces the
// same one-line git diff a hand edit would.

export interface Target {
  /** Section header the key lives under: `[name]` or `[[name]]`. Omit for top-level keys. */
  section?: string;
  /** For `[[name]]` array sections: which occurrence (0-based). */
  index?: number;
}

export function buildScalarEdit(
  document: vscode.TextDocument,
  key: string,
  value: number | boolean | string,
  target: Target = {}
): { range: vscode.Range; newText: string } | undefined {
  const { start, end } = sectionBounds(document, target);
  if (start < 0) {
    return undefined;
  }

  const keyRe = new RegExp(`^\\s*${escapeRegExp(key)}\\s*=\\s*`);
  for (let line = start; line < end; line++) {
    const text = document.lineAt(line).text;
    const m = keyRe.exec(text);
    if (!m) {
      continue;
    }

    // Value runs from the end of "key = " to the start of an inline comment
    // (or end of line), minus trailing spaces we leave untouched.
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

// applyScalarEdit performs the edit and saves, so the engine's file watcher
// (which sees the disk, not VS Code buffers) picks the change up immediately —
// that's what makes a fader drag audible without a manual save.
export async function applyScalarEdit(
  document: vscode.TextDocument,
  key: string,
  value: number | boolean | string,
  target: Target = {}
): Promise<boolean> {
  const edit = buildScalarEdit(document, key, value, target);
  if (!edit) {
    return false;
  }
  const we = new vscode.WorkspaceEdit();
  we.replace(document.uri, edit.range, edit.newText);
  if (!(await vscode.workspace.applyEdit(we))) {
    return false;
  }
  return document.save();
}

// sectionBounds returns the [start, end) line range to scan for the key:
// the top-level region (before any header), a named `[section]`, or the
// index-th `[[section]]` occurrence.
function sectionBounds(
  document: vscode.TextDocument,
  target: Target
): { start: number; end: number } {
  const headerRe = /^\s*(\[\[?)\s*([A-Za-z0-9_.-]+)\s*\]\]?/;

  if (!target.section) {
    // Top-level: from line 0 to the first header (or EOF).
    for (let line = 0; line < document.lineCount; line++) {
      if (headerRe.test(document.lineAt(line).text)) {
        return { start: 0, end: line };
      }
    }
    return { start: 0, end: document.lineCount };
  }

  let seen = 0;
  const wanted = target.index ?? 0;
  for (let line = 0; line < document.lineCount; line++) {
    const m = headerRe.exec(document.lineAt(line).text);
    if (!m || m[2] !== target.section) {
      continue;
    }
    const isArray = m[1] === '[[';
    const matches = isArray ? seen++ === wanted : true;
    if (!matches) {
      continue;
    }
    // Section body: from the next line to the next header of any kind.
    for (let endLine = line + 1; endLine < document.lineCount; endLine++) {
      if (headerRe.test(document.lineAt(endLine).text)) {
        return { start: line + 1, end: endLine };
      }
    }
    return { start: line + 1, end: document.lineCount };
  }
  return { start: -1, end: -1 };
}

export function formatTomlValue(v: number | boolean | string): string {
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
