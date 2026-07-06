import * as vscode from 'vscode';
import {
	DocumentEditorProvider,
	trackKind,
	busKind,
	masterKind,
	projectKind,
} from './editors/documentEditor';
import { TransportSession } from './engine/session';
import { TransportViewProvider } from './views/transportView';
import { MixerPanel } from './panels/mixerPanel';
import { ArrangementPanel } from './panels/arrangementPanel';

// Activation is registration only — all behaviour lives in the modules:
//   editors/   per-file custom editors (track, bus, master, project)
//   panels/    command-palette panels (mixer)
//   views/     sidebar views (transport)
//   engine/    the codaw process + session state
export function activate(context: vscode.ExtensionContext) {
	// Per-file editors: one generic provider, four kind configs.
	for (const kind of [trackKind, busKind, masterKind, projectKind]) {
		context.subscriptions.push(DocumentEditorProvider.register(context, kind));
	}

	// Session logic (engine process, playback state) is UI-independent; the
	// sidebar view and the commands are thin frontends over it.
	const session = new TransportSession(context);

	context.subscriptions.push(
		vscode.window.registerWebviewViewProvider(
			TransportViewProvider.viewType,
			new TransportViewProvider(context.extensionUri, session),
			{ webviewOptions: { retainContextWhenHidden: true } }
		),
		vscode.commands.registerCommand('codaw.togglePlay', () => session.togglePlay()),
		vscode.commands.registerCommand('codaw.stop', () => session.stop()),
		vscode.commands.registerCommand('codaw.openMixer', () => MixerPanel.open(context.extensionUri)),
		vscode.commands.registerCommand('codaw.openArrangement', () =>
			ArrangementPanel.open(context.extensionUri, session)
		)
	);

	// The transport view is gated on `codaw.hasProject` (see the `when` clause
	// in package.json) so it only appears in workspaces that actually contain
	// a CodaW project. Context keys are how extensions feed facts into VS
	// Code's when-clause system.
	void updateHasProject();
	const projectWatcher = vscode.workspace.createFileSystemWatcher('**/project.toml');
	projectWatcher.onDidCreate(() => void updateHasProject());
	projectWatcher.onDidDelete(() => void updateHasProject());
	context.subscriptions.push(projectWatcher);
}

async function updateHasProject(): Promise<void> {
	const found = await vscode.workspace.findFiles('**/project.toml', '**/node_modules/**', 1);
	await vscode.commands.executeCommand('setContext', 'codaw.hasProject', found.length > 0);
}

export function deactivate() {}
