import { ContainerModule } from '@theia/core/shared/inversify';
import { ConnectionContainerModule } from '@theia/core/lib/node/messaging/connection-container-module';
import { ENGINE_SERVICE_PATH, EngineClient, EngineServer } from '../common/engine-protocol.js';
import { EngineServerImpl } from './engine-server.js';

const engineConnectionModule = ConnectionContainerModule.create(({ bind, bindBackendService }) => {
    
    bind(EngineServerImpl).toSelf().inSingletonScope();
    bind(EngineServer).toService(EngineServerImpl);
    bindBackendService<EngineServer, EngineClient>(ENGINE_SERVICE_PATH, EngineServer, (server, client) => {
        server.setClient(client);
        client.onDidCloseConnection(() => server.dispose());
        return server;
    });
});

export default new ContainerModule(bind => {
    bind(ConnectionContainerModule).toConstantValue(engineConnectionModule);
});