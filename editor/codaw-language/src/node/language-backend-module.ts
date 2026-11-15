import { ContainerModule } from '@theia/core/shared/inversify';
import { ConnectionContainerModule } from '@theia/core/lib/node/messaging/connection-container-module';
import { LANGUAGE_SERVICE_PATH, LanguageClient, LanguageServer } from '../common/language-protocol.js';
import { LanguageServerImpl } from './language-server.js';

// One language server per frontend connection, like the engine: a window gets its own, and it ends with it.
const languageConnectionModule = ConnectionContainerModule.create(({ bind, bindBackendService }) => {
    bind(LanguageServerImpl).toSelf().inSingletonScope();
    bind(LanguageServer).toService(LanguageServerImpl);
    bindBackendService<LanguageServer, LanguageClient>(LANGUAGE_SERVICE_PATH, LanguageServer, (server, client) => {
        server.setClient(client);
        client.onDidCloseConnection(() => server.dispose());
        return server;
    });
});

export default new ContainerModule(bind => {
    bind(ConnectionContainerModule).toConstantValue(languageConnectionModule);
});