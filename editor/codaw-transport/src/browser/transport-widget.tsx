/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';

import { TransportService } from './transport-service.js';
import { ReactWidget, codicon } from '@theia/core/lib/browser';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';

import { CommandRegistry } from '@theia/core/lib/common/command';
import { Message } from '@theia/core/shared/@lumino/messaging';
import { TransportCommands } from './transport-commands.js';
import { ProjectCommands } from 'codaw-project/lib/browser/project-navigation';

/**
 * The transport bar: the row under the menu bar with play, stop, loop, the playhead position and the
 * tempo. It is not a view the user can move or close, because it is the one control every part of the
 * application needs; the shell places it, see `CodawApplicationShell`.
 *
 * Its buttons run commands rather than calling the transport service, so that a button, its keybinding
 * and the command palette entry all take the same path.
 */
@injectable()
export class TransportWidget extends ReactWidget {

    static readonly ID = 'codaw-transport';

    @inject(TransportService)
    protected readonly transport: TransportService;

    @inject(CommandRegistry)
    protected readonly commands: CommandRegistry;

    @postConstruct()
    protected init(): void {
        this.id = TransportWidget.ID;
        this.addClass('codaw-transport');
        this.node.setAttribute('role', 'toolbar');
        this.node.setAttribute('aria-label', 'Transport');
        this.toDispose.push(this.transport.onDidChange(() => this.update()));
        this.update();
    }

    protected onAfterAttach(msg: Message): void {
        super.onAfterAttach(msg);
        this.update();
    }

    protected render(): React.ReactNode {
        const state = this.transport.state;
        const project = this.transport.projectName;
        const beat = state.positionBeats;
        const bar = Math.floor(beat / state.beatsPerBar) + 1;
        const beatInBar = Math.floor(beat % state.beatsPerBar) + 1;
        const bpm = state.bpm === undefined ? '—' : Number.isInteger(state.bpm) ? String(state.bpm) : state.bpm.toFixed(1);
        const openTransportSettings = () =>
            this.commands.executeCommand(ProjectCommands.REVEAL_IN_FILE.id, this.transport.projectUri, { section: 'transport' });
        return <React.Fragment>
            <button type='button' className='codaw-tp-project' disabled={!this.transport.projectUri}
                title={project ? 'The project: click to open project.toml' : 'No CodaW project is open'}
                onClick={() => this.commands.executeCommand(ProjectCommands.REVEAL_IN_FILE.id, this.transport.projectUri, { section: 'project' })}>
                {project ?? 'No project'}
            </button>

            <div className='codaw-tp-buttons'>
                <button type='button' className='codaw-transport-button codaw-tp-record' disabled
                    title='Record (the engine cannot record yet)' aria-label='Record'>
                    <i className={codicon('circle-large-filled')}></i>
                </button>
                {this.renderButton(TransportCommands.TOGGLE_PLAY.id, state.playing ? 'debug-pause' : 'play',
                    state.playing ? 'Pause (Space)' : 'Play (Space)', { active: state.playing, className: 'codaw-tp-play' })}
                {this.renderButton(TransportCommands.STOP.id, 'primitive-square', 'Stop, and back to the start')}
                {this.renderButton(TransportCommands.TOGGLE_LOOP.id, 'sync', 'Loop (not in the engine yet)', { pressed: state.looping })}
            </div>

            <div className='codaw-tp-readout' aria-live='off'>
                <Field value={String(bar).padStart(3, '0')} label='Bar' title='Bar' />
                <Field value={String(beatInBar)} label='Beat' title='Beat in the bar' />
                <Field value={bpm} label='BPM' title='Tempo: click to open it in project.toml' onClick={openTransportSettings} />
                <Field value={`${state.beatsPerBar}/${state.beatUnit}`} label='Meter' title='Time signature: click to open it in project.toml'
                    onClick={openTransportSettings} />
            </div>

            <button type='button' className='codaw-transport-button codaw-tp-metronome' disabled
                title='Metronome (not in the engine yet)' aria-label='Metronome'>
                <MetronomeIcon />
            </button>

            <div className='codaw-tp-spacer'></div>

            <div className='codaw-tp-master' title='Master level (the engine does not report levels yet)'>
                <div className='codaw-tp-master-bar'><div className='codaw-tp-master-level' style={{ width: '0%' }} /></div>
                <div className='codaw-tp-master-bar'><div className='codaw-tp-master-level' style={{ width: '0%' }} /></div>
            </div>
        </React.Fragment>;
    }

    /**
     * @param options.pressed marks a toggle (loop), which is announced as pressed or not.
     * @param options.active highlights a button that reflects a running state (playing) without being a toggle.
     * @param options.className an extra class, for a button with a color of its own (play).
     */
    protected renderButton(commandId: string, icon: string, label: string,
        options: { pressed?: boolean; active?: boolean; className?: string } = {}): React.ReactNode {
        const classes = ['codaw-transport-button'];
        if (options.className) {
            classes.push(options.className);
        }
        if (options.pressed || options.active) {
            classes.push('codaw-transport-button-on');
        }
        return <button
            type='button'
            className={classes.join(' ')}
            title={label}
            aria-label={label}
            aria-pressed={options.pressed}
            disabled={!this.commands.isEnabled(commandId)}
            onClick={() => this.commands.executeCommand(commandId)}
        >
            <i className={codicon(icon)}></i>
        </button>;
    }
}

/** A readout of the display: a value, and what it is, small, underneath. Clickable when it opens something. */
function Field(props: { value: string; label: string; title: string; onClick?: () => void }): React.ReactElement {
    const content = <React.Fragment>
        <span className='codaw-tp-value'>{props.value}</span>
        <span className='codaw-tp-label'>{props.label}</span>
    </React.Fragment>;
    return props.onClick
        ? <button type='button' className='codaw-tp-field codaw-tp-field-link' title={props.title} onClick={props.onClick}>{content}</button>
        : <span className='codaw-tp-field' title={props.title}>{content}</span>;
}

/** No codicon for a metronome: a small one, drawn in the current text color. */
function MetronomeIcon(): React.ReactElement {
    return <svg width='16' height='16' viewBox='0 0 16 16' fill='none' stroke='currentColor' strokeWidth='1.3' strokeLinejoin='round'>
        <path d='M5.5 1.5h5l3 13h-11z' />
        <path d='M8 11.5l4-7' strokeLinecap='round' />
        <path d='M3.6 10.5h8.8' />
    </svg>;
}
