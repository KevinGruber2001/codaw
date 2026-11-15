/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

import * as React from '@theia/core/shared/react';

/*
 * The transport's icons, drawn for it: solid shapes on a 16-unit grid, in the current text color, in
 * the manner of a hardware transport (and Logic's). Codicons are line icons made for a code editor;
 * these read as tape-machine controls at a glance.
 */

function Icon(props: { children: React.ReactNode; title?: string }): React.ReactElement {
    return <svg className='codaw-tp-icon' width='16' height='16' viewBox='0 0 16 16' fill='currentColor' aria-hidden='true'>
        {props.children}
    </svg>;
}

export function GoToStartIcon(): React.ReactElement {
    return <Icon>
        <rect x='2.5' y='3' width='2' height='10' rx='0.5' />
        <path d='M13 3.6v8.8a.6.6 0 0 1-.92.5L5.6 8.5a.6.6 0 0 1 0-1l6.48-4.4a.6.6 0 0 1 .92.5z' />
    </Icon>;
}

export function RewindIcon(): React.ReactElement {
    return <Icon>
        <path d='M8 4.1v7.8a.55.55 0 0 1-.85.46L1.6 8.46a.55.55 0 0 1 0-.92l5.55-3.9a.55.55 0 0 1 .85.46z' />
        <path d='M14.5 4.1v7.8a.55.55 0 0 1-.85.46L8.1 8.46a.55.55 0 0 1 0-.92l5.55-3.9a.55.55 0 0 1 .85.46z' />
    </Icon>;
}

export function ForwardIcon(): React.ReactElement {
    return <Icon>
        <path d='M1.5 4.1v7.8a.55.55 0 0 0 .85.46l5.55-3.9a.55.55 0 0 0 0-.92L2.35 3.64a.55.55 0 0 0-.85.46z' />
        <path d='M8 4.1v7.8a.55.55 0 0 0 .85.46l5.55-3.9a.55.55 0 0 0 0-.92L8.85 3.64A.55.55 0 0 0 8 4.1z' />
    </Icon>;
}

export function StopIcon(): React.ReactElement {
    return <Icon><rect x='3.5' y='3.5' width='9' height='9' rx='1.2' /></Icon>;
}

export function PlayIcon(): React.ReactElement {
    return <Icon><path d='M4.5 2.9v10.2a.7.7 0 0 0 1.06.6l8.5-5.1a.7.7 0 0 0 0-1.2l-8.5-5.1a.7.7 0 0 0-1.06.6z' /></Icon>;
}

export function PauseIcon(): React.ReactElement {
    return <Icon>
        <rect x='3.5' y='3' width='3' height='10' rx='0.8' />
        <rect x='9.5' y='3' width='3' height='10' rx='0.8' />
    </Icon>;
}

export function RecordIcon(): React.ReactElement {
    return <Icon><circle cx='8' cy='8' r='5' /></Icon>;
}

/** Cycle: two arrows chasing each other around a loop, as in Logic. */
export function CycleIcon(): React.ReactElement {
    return <svg className='codaw-tp-icon' width='16' height='16' viewBox='0 0 16 16' fill='none' stroke='currentColor'
        strokeWidth='1.6' strokeLinecap='round' strokeLinejoin='round' aria-hidden='true'>
        <path d='M2.5 8.5V7.5A2.5 2.5 0 0 1 5 5h7.5' />
        <path d='M10.5 3l2 2-2 2' />
        <path d='M13.5 7.5v1A2.5 2.5 0 0 1 11 11H3.5' />
        <path d='M5.5 13l-2-2 2-2' />
    </svg>;
}

export function MetronomeIcon(): React.ReactElement {
    return <svg className='codaw-tp-icon' width='16' height='16' viewBox='0 0 16 16' fill='none' stroke='currentColor'
        strokeWidth='1.3' strokeLinejoin='round' aria-hidden='true'>
        <path d='M5.5 1.5h5l3 13h-11z' />
        <path d='M8 11.5l4-7' strokeLinecap='round' />
        <path d='M3.6 10.5h8.8' />
    </svg>;
}
