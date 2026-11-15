/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { Emitter, Event } from '@theia/core/lib/common/event';
import URI from '@theia/core/lib/common/uri';
import { FileService } from '@theia/filesystem/lib/browser/file-service';
import { FileChangesEvent } from '@theia/filesystem/lib/common/files';
import { WorkspaceService } from '@theia/workspace/lib/browser/workspace-service';
import { MonacoEditorModel } from '@theia/monaco/lib/browser/monaco-editor-model';
import { MonacoTextModelService } from '@theia/monaco/lib/browser/monaco-text-model-service';
import { CodawProject, ProjectProblem } from '../common/project-model.js';
import { MemberFiles, Toml, layoutFiles, parseTomlText, toProject } from './project-reader.js';
import { SchemaService } from './schema-service.js';

/** How long to wait after a change before reading again, so a burst of saves is read once. */
const RELOAD_DELAY_MS = 50;

/**
 * The open CodaW project: the workspace root's project.toml and the files its [layout] names. Read when
 * the workspace opens, and again whenever one of its TOML files changes, so every view shows what is on
 * disk, which is what the engine plays.
 *
 * A file that stops parsing halfway through an edit does not take its track out of the views: the last
 * version that parsed stays, with a problem saying the file is broken.
 *
 * Files open in an editor are read from the editor's document, unsaved changes included, so the views
 * show what the user sees in the editor. The engine plays what is saved; `isDirty` says where the two
 * differ.
 */
@injectable()
export class ProjectService {

    @inject(WorkspaceService)
    protected readonly workspaceService: WorkspaceService;

    @inject(FileService)
    protected readonly fileService: FileService;

    @inject(MonacoTextModelService)
    protected readonly textModels: MonacoTextModelService;

    @inject(SchemaService)
    protected readonly schemaService: SchemaService;

    protected readonly onDidChangeEmitter = new Emitter<CodawProject | undefined>();
    /** Fires whenever the project was read again, or closed. */
    readonly onDidChange: Event<CodawProject | undefined> = this.onDidChangeEmitter.event;

    protected _project?: CodawProject;
    protected projectUri?: URI;
    /** The last version of each file that parsed, by URI. */
    protected readonly lastGood = new Map<string, Toml>();
    protected reloadTimer?: ReturnType<typeof setTimeout>;
    protected reading?: Promise<void>;

    @postConstruct()
    protected init(): void {
        this.reading = this.read();
        this.workspaceService.onWorkspaceChanged(() => this.scheduleRead());
        this.fileService.onDidFilesChange(event => this.onFilesChanged(event));
        // Documents: typing in a project file, saving it, closing it (then the disk is the truth again).
        this.textModels.models.forEach(model => this.watchModel(model));
        this.textModels.onDidCreate(model => this.watchModel(model));
    }

    /** Whether a file has unsaved changes, so what the views show is not yet what the engine plays. */
    isDirty(uri: string): boolean {
        return this.textModels.get(uri)?.dirty ?? false;
    }

    protected watchModel(model: MonacoEditorModel): void {
        if (new URI(model.uri).path.ext !== '.toml') {
            return;
        }
        // The model's own events end with it, so these need no disposing.
        model.onDidChangeContent(() => this.onModelChanged(model));
        model.onDirtyChanged(() => this.onModelChanged(model));
        model.onWillDispose(() => this.onModelChanged(model));
    }

    protected onModelChanged(model: MonacoEditorModel): void {
        const root = this.projectUri?.parent;
        if (root && root.isEqualOrParent(new URI(model.uri))) {
            this.scheduleRead();
        }
    }

    /** The project, or `undefined` when the workspace is not a CodaW project. */
    get project(): CodawProject | undefined {
        return this._project;
    }

    /** Resolves once the first read is done. */
    get ready(): Promise<void> {
        return this.reading ?? Promise.resolve();
    }

    protected onFilesChanged(event: FileChangesEvent): void {
        const root = this.projectUri?.parent ?? this.workspaceService.tryGetRoots()[0]?.resource;
        if (root && event.changes.some(change => change.resource.path.ext === '.toml' && root.isEqualOrParent(change.resource))) {
            this.scheduleRead();
        }
    }

    protected scheduleRead(): void {
        clearTimeout(this.reloadTimer);
        this.reloadTimer = setTimeout(() => this.reading = this.read(), RELOAD_DELAY_MS);
    }

    protected async read(): Promise<void> {
        // The schema first: a view that shows a project can then count on the schema being there, if there is one.
        await this.schemaService.ready;
        const root = (await this.workspaceService.roots)[0]?.resource;
        const projectUri = root?.resolve('project.toml');
        if (!projectUri || !await this.fileService.exists(projectUri)) {
            this.projectUri = undefined;
            this.setProject(undefined);
            return;
        }
        this.projectUri = projectUri;
        const problems: ProjectProblem[] = [];
        const project = await this.readToml(projectUri, 'project.toml', problems);
        if (!project) {
            this.setProject(undefined);
            return;
        }

        const files = layoutFiles(project);
        const readMember = (file: string) => this.readToml(projectUri.parent.resolve(file).normalizePath(), file, problems);
        const [tracks, buses, master] = await Promise.all([
            Promise.all(files.tracks.map(async file => [file, await readMember(file)] as const)),
            Promise.all(files.buses.map(async file => [file, await readMember(file)] as const)),
            files.master ? readMember(files.master) : Promise.resolve(undefined)
        ]);
        const members: MemberFiles = {
            tracks: new Map(tracks.filter((entry): entry is [string, Toml] => !!entry[1])),
            buses: new Map(buses.filter((entry): entry is [string, Toml] => !!entry[1])),
            master
        };
        this.setProject(toProject(projectUri, project, members, problems));
    }

    /**
     * Reads and parses one file. When it does not parse, or cannot be read, records why and falls back
     * to the last version that did.
     */
    protected async readToml(uri: URI, file: string, problems: ProjectProblem[]): Promise<Toml | undefined> {
        const key = uri.toString();
        let text: string;
        try {
            // The editor's document if the file is open (unsaved changes included), the disk otherwise.
            const model = this.textModels.get(key);
            text = model ? model.textEditorModel.getValue() : (await this.fileService.read(uri)).value;
        } catch (error) {
            problems.push({ file, message: 'cannot be read: ' + (error instanceof Error ? error.message : String(error)) });
            this.lastGood.delete(key);
            return undefined;
        }
        const parsed = parseTomlText(text);
        if ('error' in parsed) {
            const previous = this.lastGood.get(key);
            problems.push({ file, message: parsed.error + (previous ? ' (showing the last version that parsed)' : '') });
            return previous;
        }
        this.lastGood.set(key, parsed.toml);
        return parsed.toml;
    }

    protected setProject(project: CodawProject | undefined): void {
        this._project = project;
        this.onDidChangeEmitter.fire(project);
    }
}
