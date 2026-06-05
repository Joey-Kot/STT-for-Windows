// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

import {
  Cancel,
  ConfirmQuit,
  GetState,
  GetUIStyle,
  GetWindowState,
  LoadConfig,
  RequestQuit,
  SaveConfig,
  SetMinimal,
  TogglePause,
  ToggleRecording
} from "./wailsjs/go/main/App.js";
import {
  EventsOn,
  WindowGetPosition,
  WindowSetMaxSize,
  WindowSetMinSize,
  WindowSetPosition,
  WindowSetSize
} from "./wailsjs/runtime/runtime.js";

const FLOATING_SIZE = { width: 222, height: 94 };
const MINIMAL_SIZE = { width: 170, height: 46 };
const SETTINGS_SIZE = { width: 680, height: 560 };

const groups = [
  {
    name: "API",
    fields: ["API_ENDPOINT", "TOKEN", "MODEL", "LANGUAGE", "PROMPT", "TEXT_PATH", "ExtraConfig"]
  },
  {
    name: "Audio",
    fields: ["CHANNELS", "SAMPLING_RATE", "SAMPLING_RATE_DEPTH", "BIT_RATE", "CODECS", "CONTAINER"]
  },
  {
    name: "Network",
    fields: ["REQUEST_TIMEOUT", "MAX_RETRY", "RETRY_BASE_DELAY", "ENABLE_HTTP2", "VERIFY_SSL"]
  },
  {
    name: "Hotkeys",
    fields: ["START_KEY", "PAUSE_KEY", "CANCEL_KEY", "HOTKEY_HOOK"]
  },
  {
    name: "Cache",
    fields: ["CACHE_DIR", "KEEP_CACHE"]
  },
  {
    name: "Notifications",
    fields: ["NOTIFICATION", "REQUEST_FAILED_NOTIFICATION"]
  },
  {
    name: "Debug",
    fields: ["FFMPEG_DEBUG", "RECORD_DEBUG", "HOTKEY_DEBUG", "UPLOAD_DEBUG"]
  },
  {
    name: "Advanced",
    fields: []
  }
];

const fieldMeta = {
  API_ENDPOINT: { label: "API endpoint", type: "url" },
  TOKEN: { label: "Token", type: "password" },
  MODEL: { label: "Model", type: "text" },
  LANGUAGE: { label: "Language", type: "text" },
  PROMPT: { label: "Prompt", type: "textarea" },
  TEXT_PATH: { label: "Text path", type: "text" },
  ExtraConfig: { label: "Extra config", type: "textarea" },
  CHANNELS: { label: "Channels", type: "number" },
  SAMPLING_RATE: { label: "Sampling rate", type: "number" },
  SAMPLING_RATE_DEPTH: { label: "Sample depth", type: "number" },
  BIT_RATE: { label: "Bit rate", type: "number" },
  CODECS: { label: "Codec", type: "text" },
  CONTAINER: { label: "Container", type: "text" },
  REQUEST_TIMEOUT: { label: "Request timeout", type: "number" },
  MAX_RETRY: { label: "Max retry", type: "number" },
  RETRY_BASE_DELAY: { label: "Retry delay", type: "number", step: "0.1" },
  ENABLE_HTTP2: { label: "HTTP/2", type: "checkbox" },
  VERIFY_SSL: { label: "Verify SSL", type: "checkbox" },
  START_KEY: { label: "Start key", type: "text" },
  PAUSE_KEY: { label: "Pause key", type: "text" },
  CANCEL_KEY: { label: "Cancel key", type: "text" },
  HOTKEY_HOOK: { label: "Low-level hook", type: "checkbox" },
  CACHE_DIR: { label: "Cache dir", type: "text" },
  KEEP_CACHE: { label: "Keep cache", type: "checkbox" },
  NOTIFICATION: { label: "Notification", type: "checkbox" },
  REQUEST_FAILED_NOTIFICATION: { label: "Request failed placeholder", type: "checkbox" },
  FFMPEG_DEBUG: { label: "FFmpeg debug", type: "checkbox" },
  RECORD_DEBUG: { label: "Record debug", type: "checkbox" },
  HOTKEY_DEBUG: { label: "Hotkey debug", type: "checkbox" },
  UPLOAD_DEBUG: { label: "Upload debug", type: "checkbox" }
};

const state = {
  runtime: { state: "Idle", message: "Idle" },
  ui: { rounded: false },
  window: { minimal: false },
  drag: {
    active: false,
    moved: false,
    pointerId: null,
    startClientX: 0,
    startClientY: 0,
    startWindowX: 0,
    startWindowY: 0
  },
  suppressRecordClick: false,
  config: {},
  activeGroup: "API"
};

const el = {
  panel: document.getElementById("app"),
  status: document.getElementById("status"),
  recordBtn: document.getElementById("recordBtn"),
  pauseBtn: document.getElementById("pauseBtn"),
  pauseIcon: document.getElementById("pauseIcon"),
  cancelBtn: document.getElementById("cancelBtn"),
  settingsBtn: document.getElementById("settingsBtn"),
  minimalBtn: document.getElementById("minimalBtn"),
  minimalIcon: document.querySelector("#minimalBtn svg"),
  floating: document.querySelector(".floating"),
  settingsView: document.getElementById("settingsView"),
  closeSettingsBtn: document.getElementById("closeSettingsBtn"),
  cancelSettingsBtn: document.getElementById("cancelSettingsBtn"),
  saveSettingsBtn: document.getElementById("saveSettingsBtn"),
  tabs: document.getElementById("tabs"),
  configForm: document.getElementById("configForm"),
  configPath: document.getElementById("configPath"),
  saveStatus: document.getElementById("saveStatus"),
  quitDialog: document.getElementById("quitDialog"),
  quitMessage: document.getElementById("quitMessage"),
  confirmQuitBtn: document.getElementById("confirmQuitBtn")
};

function setRuntime(next) {
  state.runtime = next || state.runtime;
  const runtimeState = state.runtime.state || "Idle";
  el.panel.dataset.state = runtimeState;
  el.recordBtn.dataset.mode = runtimeState.toLowerCase();
  el.status.textContent = state.runtime.error || state.runtime.message || runtimeState;
  el.recordBtn.classList.toggle("active", runtimeState === "Recording" || runtimeState === "Paused");
  el.pauseBtn.disabled = runtimeState !== "Recording" && runtimeState !== "Paused";
  el.cancelBtn.disabled = runtimeState !== "Recording" && runtimeState !== "Paused";
  el.recordBtn.disabled = runtimeState === "Uploading";

  if (runtimeState === "Paused") {
    el.pauseIcon.innerHTML = '<path d="M8 5v14l11-7Z"></path>';
  } else {
    el.pauseIcon.innerHTML = '<path d="M8 5v14"></path><path d="M16 5v14"></path>';
  }
}

function applyUIStyle(next) {
  if (next) {
    state.ui = next;
  }
  el.panel.dataset.rounded = String(state.ui.rounded);
}

function updateMinimalIcon() {
  el.minimalIcon.innerHTML = state.window.minimal
    ? '<path d="M12 5v14"></path><path d="M5 12h14"></path>'
    : '<path d="M6 12h12"></path>';
}

async function beginMinimalDrag(event) {
  if (!state.window.minimal) {
    return;
  }

  const position = await WindowGetPosition();
  state.drag.active = true;
  state.drag.moved = false;
  state.drag.pointerId = event.pointerId;
  state.drag.startClientX = event.clientX;
  state.drag.startClientY = event.clientY;
  state.drag.startWindowX = position.x;
  state.drag.startWindowY = position.y;
  el.recordBtn.setPointerCapture(event.pointerId);
}

function moveMinimalDrag(event) {
  if (!state.drag.active || state.drag.pointerId !== event.pointerId) {
    return;
  }

  const deltaX = event.clientX - state.drag.startClientX;
  const deltaY = event.clientY - state.drag.startClientY;

  if (!state.drag.moved && Math.abs(deltaX) + Math.abs(deltaY) < 4) {
    return;
  }

  state.drag.moved = true;
  WindowSetPosition(
    Math.round(state.drag.startWindowX + deltaX),
    Math.round(state.drag.startWindowY + deltaY)
  );
}

function endMinimalDrag(event) {
  if (!state.drag.active || state.drag.pointerId !== event.pointerId) {
    return;
  }

  if (el.recordBtn.hasPointerCapture(event.pointerId)) {
    el.recordBtn.releasePointerCapture(event.pointerId);
  }
  state.suppressRecordClick = state.drag.moved;
  state.drag.active = false;
  state.drag.moved = false;
  state.drag.pointerId = null;
}

async function setWindowBounds(width, height, lock = false) {
  if (!lock) {
    await WindowSetMaxSize(720, 620);
    await WindowSetMinSize(34, 34);
  }

  await WindowSetSize(width, height);
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await WindowSetSize(width, height);

  if (lock) {
    await WindowSetMinSize(width, height);
    await WindowSetMaxSize(width, height);
    await WindowSetSize(width, height);
  }
}

async function applyWindowState(next) {
  if (next) {
    state.window = next;
  }

  el.panel.dataset.minimal = String(state.window.minimal);
  updateMinimalIcon();
  if (state.window.minimal) {
    el.settingsView.classList.add("hidden");
    await setWindowBounds(MINIMAL_SIZE.width, MINIMAL_SIZE.height, true);
    return;
  }

  if (el.settingsView.classList.contains("hidden")) {
    await setWindowBounds(FLOATING_SIZE.width, FLOATING_SIZE.height, true);
  }
}

function renderTabs() {
  el.tabs.innerHTML = "";
  for (const group of groups) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "tab";
    button.textContent = group.name;
    button.dataset.active = String(group.name === state.activeGroup);
    button.addEventListener("click", () => {
      state.activeGroup = group.name;
      renderTabs();
      renderFields();
    });
    el.tabs.appendChild(button);
  }
}

function renderFields() {
  el.configForm.innerHTML = "";
  const group = groups.find((item) => item.name === state.activeGroup);
  const fields = group.name === "Advanced"
    ? Object.keys(state.config).filter((key) => !groups.some((item) => item.name !== "Advanced" && item.fields.includes(key)))
    : group.fields;

  for (const key of fields) {
    const meta = fieldMeta[key] || { label: key, type: typeof state.config[key] === "boolean" ? "checkbox" : "text" };
    const row = document.createElement("label");
    row.className = meta.type === "checkbox" ? "field checkField" : "field";
    row.htmlFor = `field-${key}`;

    const label = document.createElement("span");
    label.textContent = meta.label;
    row.appendChild(label);

    let input;
    if (meta.type === "textarea") {
      input = document.createElement("textarea");
      input.rows = key === "ExtraConfig" ? 6 : 3;
    } else {
      input = document.createElement("input");
      input.type = meta.type;
      if (meta.step) {
        input.step = meta.step;
      }
    }
    input.id = `field-${key}`;
    input.dataset.key = key;
    if (meta.type === "checkbox") {
      input.checked = Boolean(state.config[key]);
    } else {
      input.value = state.config[key] ?? "";
    }
    input.addEventListener("input", () => updateConfigValue(key, input, meta.type));
    row.appendChild(input);
    el.configForm.appendChild(row);
  }
}

function updateConfigValue(key, input, type) {
  if (type === "checkbox") {
    state.config[key] = input.checked;
  } else if (type === "number") {
    state.config[key] = input.value === "" ? 0 : Number(input.value);
  } else {
    state.config[key] = input.value;
  }
}

async function openSettings() {
  if (state.window.minimal) {
    await applyWindowState(await SetMinimal(false));
  }
  el.saveStatus.textContent = "";
  const payload = await LoadConfig();
  state.config = payload.data || {};
  el.configPath.textContent = payload.path || "";
  state.activeGroup = "API";
  renderTabs();
  renderFields();
  el.settingsView.classList.remove("hidden");
  await setWindowBounds(SETTINGS_SIZE.width, SETTINGS_SIZE.height, false);
  await WindowSetMinSize(SETTINGS_SIZE.width, SETTINGS_SIZE.height);
}

async function closeSettings() {
  el.settingsView.classList.add("hidden");
  await setWindowBounds(FLOATING_SIZE.width, FLOATING_SIZE.height, true);
}

async function saveSettings() {
  el.saveStatus.textContent = "Saving...";
  try {
    const payload = JSON.stringify(state.config, null, 2);
    const next = await SaveConfig(payload);
    setRuntime(next);
    el.saveStatus.textContent = "Saved";
    await closeSettings();
  } catch (err) {
    el.saveStatus.textContent = String(err);
  }
}

el.recordBtn.addEventListener("pointerdown", (event) => {
  beginMinimalDrag(event).catch(console.error);
});
el.recordBtn.addEventListener("pointermove", moveMinimalDrag);
el.recordBtn.addEventListener("pointerup", endMinimalDrag);
el.recordBtn.addEventListener("pointercancel", endMinimalDrag);
el.recordBtn.addEventListener("lostpointercapture", (event) => {
  if (state.drag.active && state.drag.pointerId === event.pointerId) {
    state.drag.active = false;
    state.drag.moved = false;
    state.drag.pointerId = null;
  }
});
el.recordBtn.addEventListener("click", async () => {
  if (state.suppressRecordClick) {
    state.suppressRecordClick = false;
    return;
  }
  setRuntime(await ToggleRecording());
});
el.pauseBtn.addEventListener("click", async () => setRuntime(await TogglePause()));
el.cancelBtn.addEventListener("click", async () => setRuntime(await Cancel()));
el.settingsBtn.addEventListener("click", async () => {
  await openSettings();
});
el.minimalBtn.addEventListener("click", async () => {
  await applyWindowState(await SetMinimal(!state.window.minimal));
});
el.closeSettingsBtn.addEventListener("click", closeSettings);
el.cancelSettingsBtn.addEventListener("click", closeSettings);
el.saveSettingsBtn.addEventListener("click", saveSettings);
el.confirmQuitBtn.addEventListener("click", async () => {
  await ConfirmQuit();
});

EventsOn("runtime:state", setRuntime);
EventsOn("settings:open", openSettings);
EventsOn("ui:style", applyUIStyle);
EventsOn("window:minimal", (next) => {
  applyWindowState(next).catch(console.error);
});
EventsOn("quit:confirm", (snapshot) => {
  el.quitMessage.textContent = `Current state: ${snapshot.state}.`;
  el.quitDialog.showModal();
});

el.floating.addEventListener("dblclick", async () => {
  if (state.window.minimal) {
    await applyWindowState(await SetMinimal(false));
  }
});

window.addEventListener("keydown", async (event) => {
  if (event.key === "Escape" && !el.settingsView.classList.contains("hidden")) {
    event.preventDefault();
    await closeSettings();
  }
  if (event.key === "Escape" && el.settingsView.classList.contains("hidden")) {
    const result = await RequestQuit();
    if (!result.quit) {
      el.quitMessage.textContent = result.message;
      el.quitDialog.showModal();
    }
  }
});

setRuntime(await GetState());
applyUIStyle(await GetUIStyle());
await applyWindowState(await GetWindowState());
