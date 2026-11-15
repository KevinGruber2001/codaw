import { injectable } from '@theia/core/shared/inversify';
import { ChildProcessWithoutNullStreams, spawn } from 'child_process';
import {
    CompletionItem, CompletionRequest, DidChangeTextDocumentNotification, Hover, HoverRequest, Position, DidCloseTextDocumentNotification, DidOpenTextDocumentNotification, ExitNotification,
    InitializeRequest, InitializedNotification, ProtocolConnection, PublishDiagnosticsNotification, ShutdownRequest,
    createProtocolConnection
} from 'vscode-languageserver-protocol/node.js';
import { LanguageClient, LanguageServer } from '../common/language-protocol.js';

/**
 * Runs `codaw lsp` for one frontend connection and passes LSP messages between the two: the documents
 * the editor has open go to the language server, its diagnostics come back to the frontend.
 */
@injectable()
export class LanguageServerImpl implements LanguageServer {

    protected client?: LanguageClient;
    protected child?: ChildProcessWithoutNullStreams;
    protected connection?: ProtocolConnection;
    protected starting?: Promise<void>;

    setClient(client: LanguageClient | undefined): void {
        this.client = client;
    }

    start(enginePath: string): Promise<void> {
        // Once per connection; a second call waits for the first.
        return this.starting ??= this.doStart(enginePath);
    }

    protected async doStart(enginePath: string): Promise<void> {
        const child = spawn(enginePath, ['lsp']);
        this.child = child;
        // stdout is the protocol; whatever the server logs goes to stderr, and from there to the backend's log.
        child.stderr.on('data', data => console.error(`[codaw lsp] ${String(data).trimEnd()}`));
        child.on('error', error => console.error(`[codaw lsp] could not start ${enginePath}: ${error.message}`));
        child.stdin.on('error', () => { /* the server is gone; 'error' or 'exit' says why */ });

        const connection = createProtocolConnection(child.stdout, child.stdin);
        connection.onNotification(PublishDiagnosticsNotification.type, params =>
            this.client?.onDiagnostics(params.uri, params.diagnostics));
        connection.listen();
        this.connection = connection;

        await connection.sendRequest(InitializeRequest.type, { processId: process.pid, rootUri: null, capabilities: {} });
        await connection.sendNotification(InitializedNotification.type, {});
    }

    async didOpen(uri: string, text: string, version: number): Promise<void> {
        await this.connection?.sendNotification(DidOpenTextDocumentNotification.type,
            { textDocument: { uri, languageId: 'toml', version, text } });
    }

    async didChange(uri: string, text: string, version: number): Promise<void> {
        // Full sync, as the server asks for: the whole text, every time.
        await this.connection?.sendNotification(DidChangeTextDocumentNotification.type,
            { textDocument: { uri, version }, contentChanges: [{ text }] });
    }

    async didClose(uri: string): Promise<void> {
        await this.connection?.sendNotification(DidCloseTextDocumentNotification.type, { textDocument: { uri } });
    }

    async completion(uri: string, position: Position): Promise<CompletionItem[]> {

        const result = await this.connection?.sendRequest(CompletionRequest.type, { textDocument: { uri }, position });

        if (!result) {

            return [];

        }

        // A server may answer with a plain list or a CompletionList; the frontend gets the items either way.

        return Array.isArray(result) ? result : result.items;

    }


    async hover(uri: string, position: Position): Promise<Hover | null> {

        return (await this.connection?.sendRequest(HoverRequest.type, { textDocument: { uri }, position })) ?? null;

    }


    dispose(): void {
        // The window went away: ask the server to stop, and make sure it does.
        const { connection, child } = this;
        this.connection = undefined;
        this.child = undefined;
        this.client = undefined;
        connection?.sendRequest(ShutdownRequest.type)
            .then(() => connection.sendNotification(ExitNotification.type))
            .catch(() => { /* already gone */ })
            .finally(() => connection.dispose());
        setTimeout(() => child?.kill(), 2000);
    }
}