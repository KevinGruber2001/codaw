/********************************************************************************
 * Copyright (C) 2026 Kevin Gruber.
 *
 * SPDX-License-Identifier: MIT
 ********************************************************************************/

/**
 * An audio file reduced to what a waveform needs: the loudest sample in each short stretch of time,
 * all channels together, from 0 (silence) to 1 (full scale).
 */
export interface AudioPeaks {
    /** Length of the audio, in seconds. */
    duration: number;
    /** How many peaks per second of audio. */
    peaksPerSecond: number;
    peaks: Float32Array;
}

export namespace AudioPeaks {
    /** The loudest peak between two points in time, in seconds. */
    export function between(audio: AudioPeaks, from: number, to: number): number {
        const first = Math.max(0, Math.floor(from * audio.peaksPerSecond));
        const last = Math.min(audio.peaks.length, Math.max(first + 1, Math.ceil(to * audio.peaksPerSecond)));
        let max = 0;
        for (let i = first; i < last; i++) {
            max = Math.max(max, audio.peaks[i]);
        }
        return max;
    }
}

const WAVE_FORMAT_PCM = 1;
const WAVE_FORMAT_FLOAT = 3;
const WAVE_FORMAT_EXTENSIBLE = 0xfffe;

/**
 * Reads a WAV file's peaks: PCM at 8, 16, 24 or 32 bits, or 32/64-bit float, any number of channels.
 * Chunks other than `fmt ` and `data` (LIST, JUNK, smpl, ...) are skipped. Throws, with a reason, on
 * anything else.
 */
export function decodeWavPeaks(bytes: Uint8Array, peaksPerSecond = 200): AudioPeaks {
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    const ascii = (at: number) => String.fromCharCode(bytes[at], bytes[at + 1], bytes[at + 2], bytes[at + 3]);
    if (bytes.length < 12 || ascii(0) !== 'RIFF' || ascii(8) !== 'WAVE') {
        throw new Error('not a WAV file');
    }

    let format = 0, channels = 0, sampleRate = 0, bits = 0;
    let dataStart = -1, dataLength = 0;
    for (let at = 12; at + 8 <= bytes.length;) {
        const id = ascii(at);
        const size = view.getUint32(at + 4, true);
        const body = at + 8;
        if (id === 'fmt ') {
            format = view.getUint16(body, true);
            channels = view.getUint16(body + 2, true);
            sampleRate = view.getUint32(body + 4, true);
            bits = view.getUint16(body + 14, true);
            if (format === WAVE_FORMAT_EXTENSIBLE && size >= 26) {
                format = view.getUint16(body + 24, true);   // the sub-format GUID starts with the format code
            }
        } else if (id === 'data') {
            dataStart = body;
            dataLength = Math.min(size, bytes.length - body);   // a file cut short still shows what is there
            break;
        }
        at = body + size + (size & 1);   // chunks are padded to an even length
    }
    if (dataStart < 0 || !channels || !sampleRate) {
        throw new Error('no audio data');
    }
    const read = sampleReader(view, format, bits);
    if (!read) {
        throw new Error(`unsupported WAV format (${format === WAVE_FORMAT_FLOAT ? 'float' : 'PCM'}, ${bits} bit)`);
    }

    const bytesPerSample = bits / 8;
    const frameSize = bytesPerSample * channels;
    const frames = Math.floor(dataLength / frameSize);
    const framesPerPeak = sampleRate / peaksPerSecond;
    const peaks = new Float32Array(Math.ceil(frames / framesPerPeak));
    for (let frame = 0; frame < frames; frame++) {
        const at = dataStart + frame * frameSize;
        let loudest = 0;
        for (let channel = 0; channel < channels; channel++) {
            loudest = Math.max(loudest, Math.abs(read(at + channel * bytesPerSample)));
        }
        const peak = Math.floor(frame / framesPerPeak);
        if (loudest > peaks[peak]) {
            peaks[peak] = Math.min(1, loudest);
        }
    }
    return { duration: frames / sampleRate, peaksPerSecond, peaks };
}

function sampleReader(view: DataView, format: number, bits: number): ((at: number) => number) | undefined {
    if (format === WAVE_FORMAT_PCM) {
        switch (bits) {
            case 8: return at => (view.getUint8(at) - 128) / 128;
            case 16: return at => view.getInt16(at, true) / 32768;
            case 24: return at => {
                const value = view.getUint8(at) | view.getUint8(at + 1) << 8 | view.getInt8(at + 2) << 16;
                return value / 8388608;
            };
            case 32: return at => view.getInt32(at, true) / 2147483648;
        }
    } else if (format === WAVE_FORMAT_FLOAT) {
        switch (bits) {
            case 32: return at => view.getFloat32(at, true);
            case 64: return at => view.getFloat64(at, true);
        }
    }
    return undefined;
}
