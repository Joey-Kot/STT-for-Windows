const tabs = ['API', 'Audio', 'Network', 'Hotkeys', 'Cache', 'Notifications', 'Debug', 'Advanced', 'JSON']
let status = { state: 'Idle', message: 'Starting…' }
let selectedTab = 'JSON'

const backend = () => window.go?.main?.App
const call = (name, ...args) => backend()?.[name]?.(...args) ?? Promise.resolve(name === 'LoadConfigJSON' ? '{}' : { state: 'Idle', message: 'Wails backend is not connected' })

const dot = document.querySelector('#dot')
const stateEl = document.querySelector('#state')
const msg = document.querySelector('#message')
const record = document.querySelector('#record')
const recordLabel = document.querySelector('#record-label')
const pause = document.querySelector('#pause')
const pauseIcon = document.querySelector('#pause-icon')
const pauseLabel = document.querySelector('#pause-label')
const cancel = document.querySelector('#cancel')
const backdrop = document.querySelector('#settings-backdrop')
const configJson = document.querySelector('#config-json')
const errorEl = document.querySelector('#error')
const saveWarning = document.querySelector('#save-warning')
const save = document.querySelector('#save')

function applyStatus(next) {
  status = next || status
  const state = (status.state || 'Idle').toLowerCase()
  dot.className = `dot ${state}`
  stateEl.textContent = status.state || 'Idle'
  msg.textContent = status.message || ''
  const isRecording = status.state === 'Recording' || status.state === 'Paused'
  const isPaused = status.state === 'Paused'
  const isBusy = status.state === 'Uploading'
  record.className = `record-button ${isRecording ? 'active' : ''} ${isBusy ? 'uploading' : ''}`
  record.disabled = isBusy
  recordLabel.textContent = isRecording ? 'Stop' : isBusy ? 'Uploading' : 'Record'
  pause.disabled = !isRecording
  pause.className = `pause-button ${isPaused ? 'active' : ''}`
  pauseIcon.textContent = isPaused ? '▶' : 'Ⅱ'
  pauseLabel.textContent = isPaused ? 'Resume' : 'Pause'
  cancel.disabled = !isRecording
  const canSave = status.state === 'Idle' || status.state === 'Error'
  save.disabled = !canSave
  saveWarning.classList.toggle('hidden', canSave)
}

async function openSettings() {
  errorEl.classList.add('hidden')
  document.querySelector('#config-path').textContent = await call('ConfigPath')
  configJson.value = await call('LoadConfigJSON')
  backdrop.classList.remove('hidden')
}

function closeSettings() { backdrop.classList.add('hidden') }

function renderTabs() {
  const root = document.querySelector('#tabs')
  root.innerHTML = '<strong>Settings</strong>'
  tabs.forEach((name) => {
    const button = document.createElement('button')
    button.textContent = name
    button.className = name === selectedTab ? 'selected' : ''
    button.onclick = () => { selectedTab = name; document.querySelector('#settings-title').textContent = `${name} Config`; renderTabs() }
    root.appendChild(button)
  })
}

record.onclick = async () => applyStatus(await call('ToggleRecording'))
pause.onclick = async () => applyStatus(await call('TogglePause'))
cancel.onclick = async () => applyStatus(await call('CancelRecording'))
document.querySelector('#hide').onclick = () => call('HideWindow')
document.querySelector('#settings-link').onclick = openSettings
document.querySelector('#close-settings').onclick = closeSettings
document.querySelector('#cancel-settings').onclick = closeSettings
save.onclick = async () => {
  errorEl.classList.add('hidden')
  try {
    applyStatus(await call('SaveConfigJSON', configJson.value))
    closeSettings()
  } catch (err) {
    errorEl.textContent = String(err)
    errorEl.classList.remove('hidden')
  }
}

renderTabs()
call('GetStatus').then(applyStatus).catch((err) => applyStatus({ state: 'Error', message: String(err) }))
window.runtime?.EventsOn?.('runtime:status', applyStatus)
window.runtime?.EventsOn?.('settings:open', openSettings)
