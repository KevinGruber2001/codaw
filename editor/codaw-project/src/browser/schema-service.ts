/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { EngineService } from 'codaw-engine/lib/browser/engine-service';
import { ProjectSchema, SCHEMA_VERSION } from '../common/project-schema.js';

/**
 * What project files may contain, asked of the engine binary once, when the application starts.
 *
 * `schema` is undefined until the answer is there, and stays undefined if there is none: the binary is
 * missing, or too old to know `codaw schema`. The views then do without ranges and units; why is in the
 * engine's log.
 */
@injectable()
export class SchemaService {

    @inject(EngineService)
    protected readonly engine: EngineService;

    protected _schema?: ProjectSchema;
    protected loading: Promise<void>;

    @postConstruct()
    protected init(): void {
        this.loading = this.load();
    }

    get schema(): ProjectSchema | undefined {
        return this._schema;
    }

    /** Resolves once the engine has answered, or failed to. The project is read after this. */
    get ready(): Promise<void> {
        return this.loading;
    }

    protected async load(): Promise<void> {
        try {
            const schema = await this.engine.schema();
            if (ProjectSchema.is(schema)) {
                this._schema = schema;
            } else {
                const version = (schema as { version?: unknown } | undefined)?.version;
                this.engine.onLog(`[schema] the engine's schema is version ${version}, the editor reads ${SCHEMA_VERSION}: `
                    + 'ranges and units are not shown. Update the one that is older.');
            }
        } catch (error) {
            this.engine.onLog(`[schema] ${error instanceof Error ? error.message : String(error)}: ranges and units are not shown.`);
        }
    }
}
