import type { CompletionItem, Diagnostic, Hover, Position } from 'vscode-languageserver-protocol';

export const LANGUAGE_SERVICE_PATH = '/services/codaw-language';

/** Backend → frontend: what the language server found. Implemented in the browser. */
export interface LanguageClient {
    onDiagnostics(uri: string, diagnostics: Diagnostic[]): void;
}

/** Frontend → backend: the language server, `codaw lsp`, behind the backend. Implemented in node. */
export const LanguageServer = Symbol('LanguageServer');
export interface LanguageServer {
    start(enginePath: string): Promise<void>;
    didOpen(uri: string, text: string, version: number): Promise<void>;
    didChange(uri: string, text: string, version: number): Promise<void>;
    didClose(uri: string): Promise<void>;
    /** What fits at a position: keys, values, section headers. */
    completion(uri: string, position: Position): Promise<CompletionItem[]>;
    /** What the key at a position is; `null` where there is nothing to say. */
    hover(uri: string, position: Position): Promise<Hover | null>;
    setClient(client: LanguageClient | undefined): void;
    dispose(): void;
}