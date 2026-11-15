import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { FrontendApplicationContribution } from '@theia/core/lib/browser/frontend-application-contribution';
import { RemoteConnectionProvider, ServiceConnectionProvider } from '@theia/core/lib/browser/messaging/service-connection-provider';
import { PreferenceService } from '@theia/core/lib/common/preferences';
import * as monaco from '@theia/monaco-editor-core';
import { MonacoEditorModel } from '@theia/monaco/lib/browser/monaco-editor-model';
import { MonacoTextModelService } from '@theia/monaco/lib/browser/monaco-text-model-service';
import type { CompletionItem, Diagnostic, Hover, MarkedString, MarkupContent } from 'vscode-languageserver-protocol';
import { ENGINE_PATH_PREFERENCE } from 'codaw-engine/lib/browser/engine-preferences';
import { LANGUAGE_SERVICE_PATH, LanguageClient, LanguageServer } from '../common/language-protocol.js';
import { TOML_LANGUAGE_ID } from './toml-language-contribution.js';

/** Who the markers are from, so they can be replaced without touching anyone else's. */
const MARKER_OWNER = 'codaw';

/**
 * The editor's side of CodaW's language server: tells it which TOML documents are open and what is in
 * them, and shows what it finds as markers in the editor (squiggles, with the message on hover).
 */
@injectable()
export class CodawLanguageClient implements LanguageClient, FrontendApplicationContribution {

    @inject(RemoteConnectionProvider)
    protected readonly connectionProvider: ServiceConnectionProvider;

    @inject(MonacoTextModelService)
    protected readonly textModels: MonacoTextModelService;

    @inject(PreferenceService)
    protected readonly preferences: PreferenceService;

    protected server: LanguageServer;
    /** The last diagnostics per document, also for documents not open yet: shown when they open. */
    protected readonly diagnostics = new Map<string, Diagnostic[]>();

    @postConstruct()
    protected init(): void {
        this.server = this.connectionProvider.createProxy<LanguageServer>(LANGUAGE_SERVICE_PATH, this);
    }

    onStart(): void {
        // Not awaited: the application does not wait for the language server.
        this.startServer();
    }

    protected async startServer(): Promise<void> {
        await this.preferences.ready;
        // The same binary as the engine: `codaw lsp` instead of `codaw serve`.
        await this.server.start(this.preferences.get<string>(ENGINE_PATH_PREFERENCE, 'codaw'));
        this.textModels.models.forEach(model => this.follow(model));
        this.textModels.onDidCreate(model => this.follow(model));
        this.registerProviders();
    }

    /** Keeps the server up to date with one document: opened, every change, closed. */
    protected follow(model: MonacoEditorModel): void {
        if (model.languageId !== TOML_LANGUAGE_ID) {
            return;
        }
        const uri = model.uri;
        const textModel = model.textEditorModel;
        this.server.didOpen(uri, textModel.getValue(), textModel.getVersionId());
        this.showMarkers(uri);
        model.onDidChangeContent(() => this.server.didChange(uri, textModel.getValue(), textModel.getVersionId()));
        model.onWillDispose(() => this.server.didClose(uri));
    }

    /**

     * Completion and hover in TOML editors: Monaco asks, the language server answers, through the backend.

     * LSP counts lines and columns from 0, Monaco from 1; these convert between the two.

     */

    protected registerProviders(): void {

        monaco.languages.registerCompletionItemProvider(TOML_LANGUAGE_ID, {

            // As the server announces: a quote opens a value, "=" starts one, "[" a header.

            triggerCharacters: ['"', '=', '[', '.'],

            provideCompletionItems: async (model, position) => {

                const items = await this.server.completion(model.uri.toString(), { line: position.lineNumber - 1, character: position.column - 1 });

                const word = model.getWordUntilPosition(position);

                const fallback = new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn);

                return { suggestions: items.map(item => this.toSuggestion(item, fallback)) };

            }

        });

        monaco.languages.registerHoverProvider(TOML_LANGUAGE_ID, {

            provideHover: async (model, position) => {

                const hover = await this.server.hover(model.uri.toString(), { line: position.lineNumber - 1, character: position.column - 1 });

                return hover ? this.toHover(hover) : null;

            }

        });

    }


    protected toSuggestion(item: CompletionItem, fallback: monaco.IRange): monaco.languages.CompletionItem {

        const edit = item.textEdit && 'range' in item.textEdit ? item.textEdit : undefined;

        return {

            label: item.label,

            kind: COMPLETION_KINDS[item.kind ?? 0] ?? monaco.languages.CompletionItemKind.Text,

            detail: item.detail,

            documentation: toMarkdown(item.documentation),

            sortText: item.sortText,

            insertText: edit?.newText ?? item.insertText ?? item.label,

            range: edit ? toMonacoRange(edit.range) : fallback

        };

    }


    protected toHover(hover: Hover): monaco.languages.Hover {

        const contents = Array.isArray(hover.contents) ? hover.contents : [hover.contents];

        return {

            contents: contents.map(content => toMarkdown(content)!),

            range: hover.range && toMonacoRange(hover.range)

        };

    }


    // ── LanguageClient: called by the backend ──

    onDiagnostics(uri: string, diagnostics: Diagnostic[]): void {
        this.diagnostics.set(uri, diagnostics);
        this.showMarkers(uri);
    }

    protected showMarkers(uri: string): void {
        const model = this.textModels.get(uri);
        if (!model) {
            return;
        }
        // LSP counts lines and columns from 0, Monaco from 1.
        monaco.editor.setModelMarkers(model.textEditorModel, MARKER_OWNER, (this.diagnostics.get(uri) ?? []).map(d => ({
            startLineNumber: d.range.start.line + 1,
            startColumn: d.range.start.character + 1,
            endLineNumber: d.range.end.line + 1,
            endColumn: d.range.end.character + 1,
            message: d.message,
            source: d.source,
            severity: d.severity === 2 ? monaco.MarkerSeverity.Warning : monaco.MarkerSeverity.Error
        })));
    }
}

/** LSP's completion kinds, for the few the server uses, as Monaco's. */
const COMPLETION_KINDS: Record<number, monaco.languages.CompletionItemKind> = {
    9: monaco.languages.CompletionItemKind.Module,      // a section header
    10: monaco.languages.CompletionItemKind.Property,   // a key
    12: monaco.languages.CompletionItemKind.Value       // a value
};

function toMonacoRange(range: { start: { line: number; character: number }; end: { line: number; character: number } }): monaco.IRange {
    return new monaco.Range(range.start.line + 1, range.start.character + 1, range.end.line + 1, range.end.character + 1);
}

/** LSP's documentation (plain text, markdown, or a marked code string) as Monaco's markdown. */
function toMarkdown(content: string | MarkupContent | MarkedString | undefined): monaco.IMarkdownString | undefined {
    if (content === undefined) {
        return undefined;
    }
    if (typeof content === 'string') {
        return { value: content };
    }
    if ('kind' in content) {
        return { value: content.value };
    }
    return { value: '```' + content.language + '\n' + content.value + '\n```' };
}
