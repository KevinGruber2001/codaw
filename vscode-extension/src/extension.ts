import * as vscode from 'vscode';
import { TrackEditorProvider } from './trackEditor';
import { TransportSession } from './transport';
import { TransportViewProvider } from './transportView';

export function activate(context: vscode.ExtensionContext) {
	const trackEditor = vscode.window.registerCustomEditorProvider(
		'codaw.trackEditor',
		new TrackEditorProvider(context),
		// Keep the webview alive when the tab is backgrounded, so knob/fader
		// state and scroll position survive tab switches.
		{ webviewOptions: { retainContextWhenHidden: true } }
	);

	// Session logic (engine process, playback state) is UI-independent; the
	// sidebar view and the commands are both thin frontends over it.
	const session = new TransportSession(context);

	const transportView = vscode.window.registerWebviewViewProvider(
		TransportViewProvider.viewType,
		new TransportViewProvider(session),
		// Keep the webview's DOM when the section is collapsed — cheap for a
		// tiny view, and reopening feels instant.
		{ webviewOptions: { retainContextWhenHidden: true } }
	);

	context.subscriptions.push(
		trackEditor,
		transportView,
		vscode.commands.registerCommand('codaw.togglePlay', () => session.togglePlay()),
		vscode.commands.registerCommand('codaw.stop', () => session.stop())
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
