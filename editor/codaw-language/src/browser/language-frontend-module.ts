import { ContainerModule } from "@theia/core/shared/inversify";
import { TomlLanguageContribution } from "./toml-language-contribution.js";
import { LanguageGrammarDefinitionContribution } from "@theia/monaco/lib/browser/textmate";
import { CodawLanguageClient } from "./language-client.js";
import { FrontendApplicationContribution } from "@theia/core/lib/browser";

export default new ContainerModule(bind => {
    bind(TomlLanguageContribution).toSelf().inSingletonScope();
    bind(LanguageGrammarDefinitionContribution).toService(TomlLanguageContribution);

    bind(CodawLanguageClient).toSelf().inSingletonScope();
    bind(FrontendApplicationContribution).toService(CodawLanguageClient);
})