// Display metadata for FX parameters. The TOML schema is intentionally open
// (params are a free-form map — see FX.UnmarshalTOML in the Go code), so the
// UI needs its own notion of sensible ranges per parameter name. Unknown
// params still render, with a conservative fallback range.

export interface ParamSpec {
  min: number;
  max: number;
  step: number;
  unit: string;
  /** Logarithmic feel for frequency-type params. */
  log?: boolean;
}

const BY_SUFFIX: [string, ParamSpec][] = [
  ['_hz', { min: 20, max: 20000, step: 1, unit: 'Hz', log: true }],
  ['_db', { min: -24, max: 24, step: 0.1, unit: 'dB' }],
  ['_ms', { min: 0, max: 500, step: 1, unit: 'ms' }],
];

const BY_NAME: Record<string, ParamSpec> = {
  wet: { min: 0, max: 1, step: 0.01, unit: '' },
  dry: { min: 0, max: 1, step: 0.01, unit: '' },
  room_size: { min: 0, max: 1, step: 0.01, unit: '' },
  ratio: { min: 1, max: 20, step: 0.1, unit: ':1' },
  threshold: { min: -60, max: 0, step: 0.5, unit: 'dB' },
};

export function paramSpec(name: string): ParamSpec {
  if (BY_NAME[name]) {
    return BY_NAME[name];
  }
  for (const [suffix, spec] of BY_SUFFIX) {
    if (name.endsWith(suffix)) {
      return spec;
    }
  }
  return { min: 0, max: 1, step: 0.01, unit: '' };
}

export function formatParam(value: number, spec: ParamSpec): string {
  const digits = spec.step >= 1 ? 0 : spec.step >= 0.1 ? 1 : 2;
  return `${value.toFixed(digits)}${spec.unit ? ' ' + spec.unit : ''}`;
}
