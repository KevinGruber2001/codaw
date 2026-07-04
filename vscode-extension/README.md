# CodaW

A VS Code extension for [CodaW](https://github.com/KevinGruber2001/codaw) — a
code-first digital audio workstation where your whole project lives in
plain-text TOML files.

Instead of hunting through mixer pages, you edit your session the way you edit
code: open a track file, tweak its settings, and the running audio engine picks
up the change live. This extension adds focused visual editors on top of those
files, so navigation is just file navigation (fuzzy open, search, the sidebar).

## Status

Early work in progress. Currently provides a custom editor for track files
(`tracks/*.toml`). More editors (buses, master, project) and a transport/mixer
UI are planned.

## Requirements

The [`codaw` engine](https://github.com/KevinGruber2001/codaw) handles audio
playback and rendering. The extension edits the project's TOML files; the engine
plays whatever those files describe.

## How it fits together

CodaW keeps the project as versionable TOML. This extension is just another way
to produce those files — anything it can't do yet, you can still edit by hand.
