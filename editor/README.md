# CodaW editor

The CodaW desktop and browser application, built on [Eclipse Theia](https://theia-ide.org).

The audio engine lives in [`../codaw`](../codaw); this folder is the editor around it. Today it is the
Theia shell with CodaW's branding, a transport bar, and placeholders for the arrangement, mixer and other
views.

## Layout

| Path | What |
|------|------|
| `codaw-product/` | The product: branding, welcome page, About dialog, Help menu, window icon; the application shell (the arrangement as the fixed main view, Theia's main area beside it as a one-file slot); the default layout; and what is left out of Theia (Outline, Problems, the Selection and Go menus). Depends on every feature package. |
| `codaw-transport/` | The transport bar between the menu and the work area, and its commands and keys. |
| `codaw-arrangement/` | The arrangement: tracks, clips, automation and the playhead. Always there, placed by the shell; clicking a track, clip or lane opens it in its file. |
| `codaw-library/` | The library view (left). |
| `codaw-inspector/` | The Inspector (right): Theia's property view (`@theia/property-view`) under CodaW's name, with what the files do not say about the selection (for a clip: lengths, the audio, the signal path). |
| `codaw-mixer/` | The mixer view (bottom). |
| `codaw-clip-editor/` | The clip editor view (bottom). |
| `codaw-engine/` | The audio engine: starts `codaw serve` for the open project (backend), `EngineService` for the frontend, and the Engine Log view (bottom). |
| `codaw-project/` | The open project, read from its TOML and read again when the files change (`ProjectService`); the peaks of its audio files, for waveforms (`AudioPeaksService`). No views. |
| `configs/base.tsconfig.json` | The compiler options every package's `tsconfig.json` extends. |
| `browser-app/` | The browser application — the Theia frontend and backend served over HTTP. |
| `electron-app/` | The desktop application, plus the `electron-builder` packaging configuration. |
| `scripts/gen-icons.py` | Renders every application icon from `../assets/logo.svg`. |

Every feature is its own Theia extension package, owning its views, commands, services and styles,
the way Theia's own packages do. Features don't depend on each other; they talk through the shared
packages `codaw-engine` and `codaw-project`. Where a view goes is the feature's call (its view contribution); which views make up the first
start is the product's (`codaw-layout-contribution.ts`). The views are placeholders for now, styled by
`codaw-product/src/browser/style/placeholder.css`.

`browser-app` and `electron-app` are mostly generated: only `package.json`, `esbuild.mjs`,
`electron-builder.yml` and the `resources`/`ico` folders are checked in, everything else is build output.

## Running

    npm install                 # also compiles every package, in dependency order
    npm run build:engine        # the audio engine, from ../codaw (needs Go), into ../codaw/bin

    npm run build:browser && npm run start:browser   # then open http://localhost:3000
    npm run build:electron && npm run start:electron

Open a CodaW project as the folder (File → Open Folder, e.g. `../codaw/testdata/basic`): the engine starts
on its own, and the status bar turns to "Engine live". The start scripts put `../codaw/bin` on the
PATH, which is where the `codaw.engine.path` setting looks by default; set it to a binary elsewhere to use
that one. What the engine says is in the Engine Log; clicking the engine status restarts it.

Pass options to Theia after `--`, e.g. `npm run start:browser -- --port=3010`.

Or launch `Start Browser Backend` / `Start Electron Backend` from VS Code.

For development, watch all packages and restart only the backend on a change:

    npm run watch:browser     # in one terminal
    npm run start:browser     # in another

## Building the desktop application on this machine

`theia rebuild:electron` runs `node-gyp`, which picks up whatever `python3` is first on the `PATH`.
The Homebrew `python@3.15` here is broken for that purpose — its `pyexpat` is linked against a newer
`libexpat` than macOS ships, so `node-gyp` dies with `Symbol not found: _XML_Set...`. Point it at the
system Python instead:

    npm_config_python=/usr/bin/python3 npm run build:electron

## Packaging the desktop application

    npm --prefix electron-app run package

Installers and archives are written to `electron-app/dist`. The macOS build is unsigned
(`-c.mac.identity=null`); signing and notarization need certificates and are not set up yet.

## Branding

Everything that carries the product name or the mark is in one place:

- **Application name, window title, config folder** — the `theia` section of each application's
  `package.json` (`applicationName`, `backend.config.configurationFolder: ".codaw"`).
- **Welcome page, About dialog, Help menu, in-application icon** — `codaw-product`. The copy that
  describes CodaW is in `src/browser/branding-util.tsx` and is shared by the welcome page and the About
  dialog so the two cannot drift apart.
- **Icons** — all generated from `../assets/logo.svg` by `npm run icons` (macOS only: it uses `qlmanage`
  and `iconutil`). The generated files are committed, so a build elsewhere never runs it. Re-run it after
  changing the logo.
- **Splash screen and loading screen** — `electron-app/resources/codaw-splash.svg` (the window Electron
  shows before the frontend is up) and `resources/preload.html` in both applications (the markup the
  frontend replaces once it has loaded).
- **Accent color** — `#FF6A5A`, defined once as `--codaw-branding-accent` in
  `codaw-product/src/browser/style/index.css`.

## Why the welcome page is written from scratch

The obvious way to get a welcome page is to override Theia's `GettingStartedWidget`, the way the Theia IDE
does. That is not done here, because `@theia/getting-started` depends on `@theia/plugin-ext` for its
walkthroughs, and Theia's application generator pulls in *every* transitively reachable package that
declares `theiaExtensions` — there is no way to exclude one. That single dependency takes the application
from 16 extensions to 35, adding the VS Code plugin host, the debugger, notebooks, tests, telemetry and the
whole `@theia/ai-*` stack, all of them contributing views and menus to a product that wants none of them.

So `codaw-product` brings its own `CodawWelcomeWidget`, a plain `ReactWidget` that needs nothing but a few
commands and the workspace service. What it gives up is Theia's walkthroughs and the
`workbench.startupEditor` preference, replaced by `codaw.welcome.showOnStartup`.

`@theia/keymaps` is a deliberate exception: the Settings section links to the keyboard shortcuts editor,
and it is the only extension that dependency adds.

Should the VS Code plugin host ever be wanted after all, adding `@theia/plugin-ext` brings those 18
extensions back whether or not the welcome page is Theia's — the welcome page is not what costs them.

The product extension is adapted from the [Eclipse Theia IDE](https://github.com/eclipse-theia/theia-ide)'s
own product extension, which is MIT licensed; the files that follow its structure say so in their header.
None of its artwork or naming is used: Eclipse Theia is a trademark of the Eclipse Foundation.
