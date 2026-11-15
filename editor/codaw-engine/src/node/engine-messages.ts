export enum EngineCommandType {
    Play = 'play',
    Stop = 'stop',
    Seek = 'seek',
    Ping = 'ping'
}

export enum EngineMessageType {
    Hello = 'hello',
    Ok = 'ok',
    Error = 'error',
    Position = 'position'
}

export type EngineCommand =
    | { type: EngineCommandType.Play }
    | { type: EngineCommandType.Stop }
    | { type: EngineCommandType.Seek; beat: number }
    | { type: EngineCommandType.Ping };

export type EngineMessage =
    | { type: EngineMessageType.Hello; protocol_version: number }
    | { type: EngineMessageType.Ok; id: number }
    | { type: EngineMessageType.Error; id?: number; message: string }
    | { type: EngineMessageType.Position; beat: number; playing: boolean };

