/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { inject, injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget, codicon } from '@theia/core/lib/browser/widgets';
import { CommandService } from '@theia/core/lib/common/command';
import { ProjectService } from 'codaw-project/lib/browser/project-service';
import { ProjectEditService } from 'codaw-project/lib/browser/project-edit-service';
import { SchemaService } from 'codaw-project/lib/browser/schema-service';
import { ProjectCommands } from 'codaw-project/lib/browser/project-navigation';
import { DbRange, Strip, faderDb, faderPosition, faderScale, formatDb, gainRange, stripsOf } from './mixer-model.js';

const ICONS: Record<Strip['kind'], string> = {
    track: 'pulse',
    bus: 'merge',
    master: 'unmute'
};

/** Pixels of vertical drag for the pan knob's full range. */
const PAN_DRAG_RANGE = 120;

/** A value being dragged, shown at once; written to the file on release. */
interface Gesture {
    key: string;
    control: 'gain' | 'pan';
    value: number;
    startY: number;
    startValue: number;
    /** The fader's range, for a gain. */
    range: DbRange;
}

/**
 * The mixer: a channel strip per track, bus and the master, each showing its file's level, pan, mute and
 * solo, where its signal goes and which effects it has. A gesture changes the strip at once and the file
 * when it ends (one edit per drag, like a hand edit), so the engine hears the new level as soon as the
 * fader is let go. Clicking a strip's name opens its file.
 *
 * Meters stay at rest for now: the engine does not report levels yet.
 */
@injectable()
export class MixerWidget extends ReactWidget {

    static readonly ID = 'codaw.mixer';
    static readonly LABEL = 'Mixer';
    static readonly ICON = 'settings';

    @inject(ProjectService)
    protected readonly projectService: ProjectService;

    @inject(ProjectEditService)
    protected readonly edits: ProjectEditService;

    @inject(SchemaService)
    protected readonly schemaService: SchemaService;

    @inject(CommandService)
    protected readonly commands: CommandService;

    protected gesture?: Gesture;
    /** Values let go of but not yet read back from the file, so a fader does not jump back meanwhile. */
    protected readonly pending = new Map<string, number>();

    @postConstruct()
    protected init(): void {
        this.id = MixerWidget.ID;
        this.title.label = MixerWidget.LABEL;
        this.title.caption = MixerWidget.LABEL;
        this.title.iconClass = codicon(MixerWidget.ICON);
        this.title.closable = true;
        this.addClass('codaw-mixer');
        this.toDispose.push(this.projectService.onDidChange(() => {
            this.pending.clear();
            this.update();
        }));
        this.update();
    }

    protected render(): React.ReactNode {
        const project = this.projectService.project;
        if (!project) {
            return <div className='codaw-mixer-empty'>No CodaW project is open.</div>;
        }
        const strips = stripsOf(project);
        return <div className='codaw-mixer-strips'>
            <div className='codaw-mixer-legend'>
                <span className='codaw-mixer-legend-head' />
                <span>Name</span>
                <span>Output</span>
                <span>Effects</span>
                <span>Pan</span>
                <span>Volume</span>
                <span className='codaw-mixer-legend-fill' />
                <span>Mute</span>
            </div>
            {strips.map(strip => this.renderStrip(strip))}
        </div>;
    }

    protected renderStrip(strip: Strip): React.ReactNode {
        const gain = this.valueOf(strip, 'gain', strip.gain);
        const range = this.rangeOf(strip);
        const pan = strip.pan === undefined ? undefined : this.valueOf(strip, 'pan', strip.pan);
        const color = strip.color !== undefined ? `var(--codaw-track-color-${strip.color % 8})` : undefined;
        const style = { '--codaw-strip-color': color ?? 'var(--theia-descriptionForeground)' } as React.CSSProperties;
        const dirty = this.projectService.isDirty(strip.uri);
        return <div key={strip.key} className={`codaw-strip codaw-strip-${strip.kind}${strip.mute ? ' codaw-strip-muted' : ''}`} style={style}>
            <div className='codaw-strip-head'>
                <i className={`${codicon(ICONS[strip.kind])} codaw-strip-icon`} />
            </div>
            <button type='button' className='codaw-strip-name' title={`${strip.file}: click to open`} onClick={() => this.reveal(strip)}>
                {strip.name}{dirty && <span className='codaw-strip-dirty'> ●</span>}
            </button>
            <div className='codaw-strip-output' title={`Goes to: ${strip.output}`}>{strip.output}</div>
            <div className='codaw-strip-fx'>
                {strip.fx.length === 0 && <span className='codaw-strip-fx-none'>—</span>}
                {strip.fx.map((fx, index) => <button type='button' key={index} className='codaw-strip-fx-chip'
                    title={`${fx.type}: click to open its settings`} onClick={() => this.reveal(strip, index)}>
                    {fx.type.replace(/_/g, ' ')}
                </button>)}
            </div>
            <div className='codaw-strip-pan'>
                {pan !== undefined
                    ? <Knob value={pan} title={`Pan ${formatPan(pan)} (drag up or down, double-click to centre)`}
                        onPointerDown={e => this.startGesture(e, strip, 'pan', pan)}
                        onPointerMove={this.moveGesture} onPointerUp={this.endGesture}
                        onDoubleClick={() => this.commit(strip, 'pan', 0)} />
                    : <span className='codaw-strip-pan-none' />}
            </div>
            <div className='codaw-strip-values'>
                <span className='codaw-strip-value' title='Level, in dB'>{formatDb(gain, range)}</span>
                <span className='codaw-strip-value codaw-strip-peak' title='Peak (the engine does not report levels yet)'>-∞</span>
            </div>
            <div className='codaw-strip-fader-area'>
                <div className='codaw-strip-fader' title={`${formatDb(gain, range)} dB (drag, double-click for 0 dB)`}
                    onPointerDown={e => this.startGesture(e, strip, 'gain', gain)}
                    onPointerMove={this.moveGesture} onPointerUp={this.endGesture}
                    onDoubleClick={() => this.commit(strip, 'gain', 0)}>
                    <div className='codaw-strip-fader-groove' />
                    <div className='codaw-strip-fader-cap' style={{ bottom: `${faderPosition(gain, range) * 100}%` }} />
                </div>
                <div className='codaw-strip-meters' title='Level (the engine does not report levels yet)'>
                    <div className='codaw-strip-meter'><div className='codaw-strip-meter-level' style={{ height: '0%' }} /></div>
                    <div className='codaw-strip-meter'><div className='codaw-strip-meter-level' style={{ height: '0%' }} /></div>
                </div>
                <div className='codaw-strip-scale'>
                    {faderScale(range).map(db => <span key={db} style={{ bottom: `${faderPosition(db, range) * 100}%` }}>{db}</span>)}
                </div>
            </div>
            <div className='codaw-strip-buttons'>
                {strip.kind === 'track' && <React.Fragment>
                    <button type='button' className={`codaw-strip-toggle codaw-strip-mute${strip.mute ? ' codaw-strip-on' : ''}`}
                        title='Mute' aria-pressed={strip.mute} onClick={() => this.edits.setMute(strip.uri, !strip.mute)}>M</button>
                    <button type='button' className={`codaw-strip-toggle codaw-strip-solo${strip.solo ? ' codaw-strip-on' : ''}`}
                        title='Solo' aria-pressed={strip.solo} onClick={() => this.edits.setSolo(strip.uri, !strip.solo)}>S</button>
                </React.Fragment>}
            </div>
        </div>;
    }

    /** What a control shows: the value being dragged, one just let go of, or the file's. */
    protected valueOf(strip: Strip, control: Gesture['control'], fromFile: number): number {
        if (this.gesture?.key === strip.key && this.gesture.control === control) {
            return this.gesture.value;
        }
        return this.pending.get(`${strip.key}/${control}`) ?? fromFile;
    }

    protected startGesture(event: React.PointerEvent<HTMLElement>, strip: Strip, control: Gesture['control'], value: number): void {
        if (event.button !== 0) {
            return;
        }
        event.currentTarget.setPointerCapture(event.pointerId);
        this.gesture = { key: strip.key, control, value, startY: event.clientY, startValue: value, range: this.rangeOf(strip) };
        if (control === 'gain') {
            // A click on the fader jumps there; dragging the cap moves it from where it is.
            const target = event.target as HTMLElement;
            if (!target.classList.contains('codaw-strip-fader-cap')) {
                this.gesture.value = this.faderValueAt(event, this.gesture.range);
                this.gesture.startValue = this.gesture.value;
            }
        }
        this.update();
    }

    protected readonly moveGesture = (event: React.PointerEvent<HTMLElement>): void => {
        const gesture = this.gesture;
        if (!gesture) {
            return;
        }
        if (gesture.control === 'gain') {
            const rect = event.currentTarget.getBoundingClientRect();
            const delta = (gesture.startY - event.clientY) / rect.height;
            gesture.value = faderDb(faderPosition(gesture.startValue, gesture.range) + delta, gesture.range);
        } else {
            const delta = (gesture.startY - event.clientY) / PAN_DRAG_RANGE * 2;
            gesture.value = Math.max(-1, Math.min(1, gesture.startValue + delta));
        }
        this.update();
    };

    protected readonly endGesture = (): void => {
        const gesture = this.gesture;
        if (!gesture) {
            return;
        }
        this.gesture = undefined;
        const strip = stripsOf(this.projectService.project!).find(s => s.key === gesture.key);
        if (strip && gesture.value !== (gesture.control === 'gain' ? strip.gain : strip.pan)) {
            this.commit(strip, gesture.control, gesture.value);
        } else {
            this.update();
        }
    };

    protected rangeOf(strip: Strip): DbRange {
        return gainRange(this.schemaService.schema, strip.kind);
    }

    protected faderValueAt(event: React.PointerEvent<HTMLElement>, range: DbRange): number {
        const rect = event.currentTarget.getBoundingClientRect();
        return faderDb((rect.bottom - event.clientY) / rect.height, range);
    }

    /** Writes a value to the strip's file, and shows it until the file has been read back. */
    protected commit(strip: Strip, control: Gesture['control'], value: number): void {
        this.pending.set(`${strip.key}/${control}`, value);
        this.update();
        if (control === 'gain') {
            this.edits.setGain(strip.uri, value);
        } else {
            this.edits.setPan(strip.uri, value);
        }
    }

    protected reveal(strip: Strip, fxIndex?: number): void {
        const target = fxIndex === undefined ? undefined : { section: 'fx', index: fxIndex };
        this.commands.executeCommand(ProjectCommands.REVEAL_IN_FILE.id, strip.uri, target);
    }
}

function formatPan(pan: number): string {
    if (Math.abs(pan) < 0.005) {
        return 'centre';
    }
    return `${Math.round(Math.abs(pan) * 100)}% ${pan < 0 ? 'left' : 'right'}`;
}

/** A pan knob: an arc from the top centre towards the side the signal is panned to. */
function Knob(props: {
    value: number; title: string;
    onPointerDown(event: React.PointerEvent<HTMLElement>): void;
    onPointerMove(event: React.PointerEvent<HTMLElement>): void;
    onPointerUp(event: React.PointerEvent<HTMLElement>): void;
    onDoubleClick(): void;
}): React.ReactElement {
    const radius = 11;
    const centre = 14;
    // -1 .. 1 maps to -135° .. 135° from the top.
    const angle = props.value * 135;
    const point = (degrees: number) => {
        const radians = (degrees - 90) * Math.PI / 180;
        return `${centre + radius * Math.cos(radians)} ${centre + radius * Math.sin(radians)}`;
    };
    const arc = Math.abs(angle) < 0.5 ? '' :
        `M ${point(0)} A ${radius} ${radius} 0 ${Math.abs(angle) > 180 ? 1 : 0} ${angle > 0 ? 1 : 0} ${point(angle)}`;
    const pointer = (angle - 90) * Math.PI / 180;
    return <span className='codaw-knob' title={props.title} onPointerDown={props.onPointerDown} onPointerMove={props.onPointerMove}
        onPointerUp={props.onPointerUp} onDoubleClick={props.onDoubleClick}>
        <svg width='28' height='28' viewBox='0 0 28 28'>
            <path className='codaw-knob-track' d={`M ${point(-135)} A ${radius} ${radius} 0 1 1 ${point(135)}`} />
            {arc && <path className='codaw-knob-value' d={arc} />}
            <circle className='codaw-knob-body' cx={centre} cy={centre} r={7.5} />
            <line className='codaw-knob-pointer' x1={centre} y1={centre}
                x2={centre + 6 * Math.cos(pointer)} y2={centre + 6 * Math.sin(pointer)} />
        </svg>
    </span>;
}
