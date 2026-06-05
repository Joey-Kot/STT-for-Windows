// Development fallback; Wails regenerates this file during `wails build`.
export function GetStatus() { return window.go?.main?.App?.GetStatus?.() ?? Promise.resolve({ state: 'Idle', message: 'Dev fallback' }) }
export function ToggleRecording() { return window.go?.main?.App?.ToggleRecording?.() ?? Promise.resolve({ state: 'Idle', message: 'Dev fallback' }) }
export function TogglePause() { return window.go?.main?.App?.TogglePause?.() ?? Promise.resolve({ state: 'Idle', message: 'Dev fallback' }) }
export function CancelRecording() { return window.go?.main?.App?.CancelRecording?.() ?? Promise.resolve({ state: 'Idle', message: 'Dev fallback' }) }
export function ConfigPath() { return window.go?.main?.App?.ConfigPath?.() ?? Promise.resolve('%APPDATA%/stt/config.json') }
export function LoadConfigJSON() { return window.go?.main?.App?.LoadConfigJSON?.() ?? Promise.resolve('{}') }
export function SaveConfigJSON(raw) { return window.go?.main?.App?.SaveConfigJSON?.(raw) ?? Promise.resolve({ state: 'Idle', message: 'Dev fallback' }) }
export function HideWindow() { return window.go?.main?.App?.HideWindow?.() ?? Promise.resolve() }
