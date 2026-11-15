/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';
import { injectable, postConstruct } from '@theia/core/shared/inversify';
import { ReactWidget } from '@theia/core/lib/browser/widgets';
import { PropertyDataService } from '@theia/property-view/lib/browser/property-data-service';
import { PropertyViewContentWidget } from '@theia/property-view/lib/browser/property-view-content-widget';
import { ClipProperties } from './clip-property-data-service.js';

/**
 * What the Inspector shows for a clip: what its file does not say (lengths, the audio, the signal path),
 * in rows of label and value. The file itself opens next to the arrangement when the clip is clicked.
 */
@injectable()
export class ClipPropertiesWidget extends ReactWidget implements PropertyViewContentWidget {

    static readonly ID = 'codaw-clip-properties';

    protected properties?: ClipProperties;

    @postConstruct()
    protected init(): void {
        this.id = ClipPropertiesWidget.ID;
        this.addClass('codaw-properties');
    }

    updatePropertyViewContent(dataService?: PropertyDataService, selection?: Object): void {
        dataService?.providePropertyData(selection).then(properties => {
            this.properties = properties as ClipProperties | undefined;
            this.update();
        });
    }

    protected render(): React.ReactNode {
        const p = this.properties;
        if (!p) {
            return undefined;
        }
        const { clip } = p;
        const effective = p.signalPath.reduce((sum, stage) => sum + stage.gain, 0);
        return <div>
            <h3 className='codaw-properties-title'>{p.name}</h3>
            <div className='codaw-properties-subtitle'>clip {p.clipNumber} of track <b>{p.trackId}</b> · {p.trackFile}</div>

            <Section title='Timing'>
                <Row label='Start'>{position(clip.start, p.beatsPerBar)} <Dim>beat {clip.start}</Dim></Row>
                <Row label='End'>{position(clip.end, p.beatsPerBar)} <Dim>beat {clip.end}</Dim></Row>
                <Row label='Length'>{bars(p.lengthBeats, p.beatsPerBar)} <Dim>{p.lengthSeconds.toFixed(2)} s</Dim></Row>
            </Section>

            <Section title='Audio'>
                <Row label='File'>{clip.file}</Row>
                {p.audio && <Row label='Duration'>{p.audio.duration.toFixed(3)} s</Row>}
                {p.audioError && <Row label='Problem'><span className='codaw-properties-error'>{p.audioError}</span></Row>}
                <Row label='Starts at'>{clip.offset.toFixed(3)} s <Dim>into the file</Dim></Row>
                <Row label='Loop'>{clip.loop ? 'on' : 'off'}
                    {p.audio && clip.loop && <Dim> plays {p.audio.repeats.toFixed(1)}× in this clip</Dim>}
                    {p.audio && !clip.loop && p.audio.repeats < 1 && <Dim> ends before the audio does</Dim>}
                </Row>
            </Section>

            <Section title='Signal path'>
                {p.signalPath.map(stage => <Row key={stage.stage} label={stage.stage}>{db(stage.gain)}</Row>)}
                <Row label='Effective'><b>{db(effective)}</b> <Dim>before effects</Dim></Row>
            </Section>
        </div>;
    }
}

function Section(props: { title: string; children: React.ReactNode }): React.ReactElement {
    return <section className='codaw-properties-section'>
        <div className='codaw-properties-section-title'>{props.title}</div>
        <table><tbody>{props.children}</tbody></table>
    </section>;
}

function Row(props: { label: string; children: React.ReactNode }): React.ReactElement {
    return <tr><td className='codaw-properties-label'>{props.label}</td><td>{props.children}</td></tr>;
}

function Dim(props: { children: React.ReactNode }): React.ReactElement {
    return <span className='codaw-properties-dim'>{props.children}</span>;
}

function position(beat: number, beatsPerBar: number): string {
    const bar = Math.floor(beat / beatsPerBar) + 1;
    const inBar = beat - (bar - 1) * beatsPerBar;
    return `${bar}.${Math.floor(inBar) + 1}.${Math.floor((inBar % 1) * 4) + 1}`;
}

function bars(beats: number, beatsPerBar: number): string {
    const whole = Math.floor(beats / beatsPerBar);
    const rest = beats - whole * beatsPerBar;
    return [whole ? `${whole} bar${whole === 1 ? '' : 's'}` : '', rest ? `${rest} beat${rest === 1 ? '' : 's'}` : ''].filter(Boolean).join(' ') || '0 beats';
}

function db(value: number): string {
    return `${value > 0 ? '+' : ''}${value.toFixed(1)} dB`;
}
