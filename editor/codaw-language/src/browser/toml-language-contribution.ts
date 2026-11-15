import { injectable } from "@theia/core/shared/inversify";
import { LanguageGrammarDefinitionContribution, TextmateRegistry } from "@theia/monaco/lib/browser/textmate";
import * as monaco from '@theia/monaco-editor-core';
import grammar from '../../src/browser/data/toml.tmLanguage.json' with { type: 'json' };

export const TOML_LANGUAGE_ID = 'toml';
export const TOML_SCOPE = 'source.toml';

@injectable()
export class TomlLanguageContribution implements LanguageGrammarDefinitionContribution {
    registerTextmateLanguage(registry: TextmateRegistry): void {
        monaco.languages.register({id: TOML_LANGUAGE_ID, extensions: [".toml"], aliases: ['TOML']});

        // This adds autoclosing of brackets
        monaco.languages.setLanguageConfiguration(TOML_LANGUAGE_ID, {
            comments: { lineComment: '#' },
            brackets: [['[', ']'], ['{', '}']],
            autoClosingPairs: [
                { open: '[', close: ']' },
                { open: '{', close: '}' },
                { open: '"', close: '"', notIn: ['string'] },
                { open: "'", close: "'", notIn: ['string'] }
            ],
            surroundingPairs: [
                { open: '[', close: ']' }, { open: '{', close: '}' },
                { open: '"', close: '"' }, { open: "'", close: "'" }
            ]
        });

        registry.registerTextmateGrammarScope(TOML_SCOPE, {
        async getGrammarDefinition() {
            return { format: 'json', content: grammar };
        }
        });
        registry.mapLanguageIdToTextmateGrammar(TOML_LANGUAGE_ID, TOML_SCOPE);
    }

}