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
const DISPLAY_LANGUAGE_KEY = "stt.gui.displayLanguage";
const DEFAULT_DISPLAY_LANGUAGE = "en";

const displayLanguages = [
  { value: "en", label: "English" },
  { value: "zh", label: "中文" },
  { value: "de", label: "Deutsch" },
  { value: "ja", label: "日本語" },
  { value: "fr", label: "Français" }
];

const translations = {
  en: {
    appTitle: "STT",
    settings: "Settings",
    closeSettings: "Close settings",
    startStopRecording: "Start or stop recording",
    pauseResume: "Pause or resume",
    cancelRecording: "Cancel recording",
    minimal: "Minimal",
    idle: "Idle",
    cancel: "Cancel",
    save: "Save",
    saving: "Saving...",
    saved: "Saved",
    quitTitle: "Quit STT?",
    quit: "Quit",
    currentState: "Current state: {state}.",
    additionalConfig: "Additional config",
    tabs: {
      Display: "Display",
      API: "API",
      Audio: "Audio",
      Network: "Network",
      Hotkeys: "Hotkeys",
      Cache: "Cache",
      Notifications: "Notifications",
      Debug: "Debug",
      About: "About"
    },
    fields: {
      DISPLAY_LANGUAGE: "Display Language",
      API_ENDPOINT: "API endpoint",
      TOKEN: "Token",
      MODEL: "Model",
      LANGUAGE: "Language",
      PROMPT: "Prompt",
      TEXT_PATH: "Text path",
      ExtraConfig: "Extra config",
      CHANNELS: "Channels",
      SAMPLING_RATE: "Sampling rate",
      SAMPLING_RATE_DEPTH: "Sample depth",
      BIT_RATE: "Bit rate",
      CODECS: "Codec",
      CONTAINER: "Container",
      REQUEST_TIMEOUT: "Request timeout",
      MAX_RETRY: "Max retry",
      RETRY_BASE_DELAY: "Retry delay",
      ENABLE_HTTP2: "HTTP/2",
      VERIFY_SSL: "Verify SSL",
      START_KEY: "Start key",
      PAUSE_KEY: "Pause key",
      CANCEL_KEY: "Cancel key",
      HOTKEY_HOOK: "Low-level hook",
      CACHE_DIR: "Cache dir",
      KEEP_CACHE: "Keep cache",
      NOTIFICATION: "Notification",
      REQUEST_FAILED_NOTIFICATION: "Request failed placeholder",
      FFMPEG_DEBUG: "FFmpeg debug",
      RECORD_DEBUG: "Record debug",
      HOTKEY_DEBUG: "Hotkey debug",
      UPLOAD_DEBUG: "Upload debug"
    },
    fieldHelp: {
      TOKEN: "Bearer Authentication Token. Other authentication types are not supported yet.",
      PROMPT: "Maps to the request field named prompt. If an API uses another name, configure it in Extra config.",
      TEXT_PATH: "Dot-separated JSON path used to read the transcription text from the API response. Example: results[0].alternatives[0].transcript. Use text for OpenAI-compatible APIs.",
      ExtraConfig: "Additional JSON request fields to send with the API request. Example: {\"enable_lid\":true,\"enable_itn\":true}. These fields are merged into the root request fields. Setting a built-in field to null, such as {\"prompt\":null}, removes it. Removable built-in fields: model, language, prompt.",
      NOTIFICATION: "Windows system notification. Not recommended."
    },
    about: {
      Author: "Author",
      Email: "Email",
      License: "License",
      GitHub: "GitHub"
    }
  },
  zh: {
    appTitle: "语音转文字",
    settings: "设置",
    closeSettings: "关闭设置",
    startStopRecording: "开始或停止录音",
    pauseResume: "暂停或继续",
    cancelRecording: "取消录音",
    minimal: "最小模式",
    idle: "空闲",
    cancel: "取消",
    save: "保存",
    saving: "保存中...",
    saved: "已保存",
    quitTitle: "退出 STT？",
    quit: "退出",
    currentState: "当前状态：{state}。",
    additionalConfig: "其他配置",
    tabs: {
      Display: "显示",
      API: "API",
      Audio: "音频",
      Network: "网络",
      Hotkeys: "快捷键",
      Cache: "缓存",
      Notifications: "通知",
      Debug: "调试",
      About: "关于"
    },
    fields: {
      DISPLAY_LANGUAGE: "显示语言",
      API_ENDPOINT: "API 端点",
      TOKEN: "令牌",
      MODEL: "模型",
      LANGUAGE: "语言",
      PROMPT: "提示词",
      TEXT_PATH: "文本路径",
      ExtraConfig: "额外配置",
      CHANNELS: "声道数",
      SAMPLING_RATE: "采样率",
      SAMPLING_RATE_DEPTH: "采样位深",
      BIT_RATE: "比特率",
      CODECS: "编码",
      CONTAINER: "容器",
      REQUEST_TIMEOUT: "请求超时",
      MAX_RETRY: "最大重试次数",
      RETRY_BASE_DELAY: "重试延迟",
      ENABLE_HTTP2: "HTTP/2",
      VERIFY_SSL: "验证 SSL",
      START_KEY: "开始快捷键",
      PAUSE_KEY: "暂停快捷键",
      CANCEL_KEY: "取消快捷键",
      HOTKEY_HOOK: "低级键盘钩子",
      CACHE_DIR: "缓存目录",
      KEEP_CACHE: "保留缓存",
      NOTIFICATION: "通知",
      REQUEST_FAILED_NOTIFICATION: "请求失败占位提示",
      FFMPEG_DEBUG: "FFmpeg 调试",
      RECORD_DEBUG: "录音调试",
      HOTKEY_DEBUG: "快捷键调试",
      UPLOAD_DEBUG: "上传调试"
    },
    fieldHelp: {
      TOKEN: "Bearer Authentication Token。暂不支持其他验证类型。",
      PROMPT: "对应请求字段名 prompt。如果某些 API 使用其他字段名，请在额外配置中配置。",
      TEXT_PATH: "用于从 API 响应 JSON 中读取转写文本的点分路径。示例：results[0].alternatives[0].transcript。OpenAI 兼容接口使用 text 即可。",
      ExtraConfig: "随 API 请求一起发送的额外 JSON 请求字段配置。示例：{\"enable_lid\":true,\"enable_itn\":true}。这里的字段会合并进根请求字段。将内置字段设为 null，例如 {\"prompt\":null}，等于删除该字段。支持删除的内置字段：model、language、prompt。",
      NOTIFICATION: "Windows 系统通知。不建议开启。"
    },
    about: {
      Author: "作者",
      Email: "邮箱",
      License: "许可证",
      GitHub: "GitHub"
    }
  },
  de: {
    appTitle: "STT",
    settings: "Einstellungen",
    closeSettings: "Einstellungen schließen",
    startStopRecording: "Aufnahme starten oder stoppen",
    pauseResume: "Pausieren oder fortsetzen",
    cancelRecording: "Aufnahme abbrechen",
    minimal: "Minimal",
    idle: "Bereit",
    cancel: "Abbrechen",
    save: "Speichern",
    saving: "Wird gespeichert...",
    saved: "Gespeichert",
    quitTitle: "STT beenden?",
    quit: "Beenden",
    currentState: "Aktueller Status: {state}.",
    additionalConfig: "Zusätzliche Konfiguration",
    tabs: {
      Display: "Anzeige",
      API: "API",
      Audio: "Audio",
      Network: "Netzwerk",
      Hotkeys: "Tastenkürzel",
      Cache: "Cache",
      Notifications: "Benachrichtigungen",
      Debug: "Debug",
      About: "Info"
    },
    fields: {
      DISPLAY_LANGUAGE: "Anzeigesprache",
      API_ENDPOINT: "API-Endpunkt",
      TOKEN: "Token",
      MODEL: "Modell",
      LANGUAGE: "Sprache",
      PROMPT: "Prompt",
      TEXT_PATH: "Textpfad",
      ExtraConfig: "Zusatzkonfiguration",
      CHANNELS: "Kanäle",
      SAMPLING_RATE: "Abtastrate",
      SAMPLING_RATE_DEPTH: "Abtasttiefe",
      BIT_RATE: "Bitrate",
      CODECS: "Codec",
      CONTAINER: "Container",
      REQUEST_TIMEOUT: "Anfrage-Timeout",
      MAX_RETRY: "Max. Wiederholungen",
      RETRY_BASE_DELAY: "Wiederholungsverzögerung",
      ENABLE_HTTP2: "HTTP/2",
      VERIFY_SSL: "SSL prüfen",
      START_KEY: "Starttaste",
      PAUSE_KEY: "Pausentaste",
      CANCEL_KEY: "Abbruchtaste",
      HOTKEY_HOOK: "Low-Level-Hook",
      CACHE_DIR: "Cache-Verzeichnis",
      KEEP_CACHE: "Cache behalten",
      NOTIFICATION: "Benachrichtigung",
      REQUEST_FAILED_NOTIFICATION: "Platzhalter bei Anfragefehler",
      FFMPEG_DEBUG: "FFmpeg-Debug",
      RECORD_DEBUG: "Aufnahme-Debug",
      HOTKEY_DEBUG: "Hotkey-Debug",
      UPLOAD_DEBUG: "Upload-Debug"
    },
    fieldHelp: {
      TOKEN: "Bearer Authentication Token. Andere Authentifizierungstypen werden derzeit nicht unterstützt.",
      PROMPT: "Entspricht dem Anfragefeld prompt. Wenn eine API einen anderen Namen verwendet, konfigurieren Sie ihn in Zusatzkonfiguration.",
      TEXT_PATH: "Punktgetrennter JSON-Pfad zum Auslesen des Transkriptionstextes aus der API-Antwort. Beispiel: results[0].alternatives[0].transcript. Für OpenAI-kompatible APIs genügt text.",
      ExtraConfig: "Zusätzliche JSON-Anfragefelder, die mit der API-Anfrage gesendet werden. Beispiel: {\"enable_lid\":true,\"enable_itn\":true}. Diese Felder werden in die Anfragefelder der Root-Ebene gemischt. Wenn ein integriertes Feld auf null gesetzt wird, z. B. {\"prompt\":null}, wird es entfernt. Entfernbare integrierte Felder: model, language, prompt.",
      NOTIFICATION: "Windows-Systembenachrichtigung. Nicht empfohlen."
    },
    about: {
      Author: "Autor",
      Email: "E-Mail",
      License: "Lizenz",
      GitHub: "GitHub"
    }
  },
  ja: {
    appTitle: "STT",
    settings: "設定",
    closeSettings: "設定を閉じる",
    startStopRecording: "録音を開始または停止",
    pauseResume: "一時停止または再開",
    cancelRecording: "録音をキャンセル",
    minimal: "最小表示",
    idle: "待機中",
    cancel: "キャンセル",
    save: "保存",
    saving: "保存中...",
    saved: "保存しました",
    quitTitle: "STT を終了しますか？",
    quit: "終了",
    currentState: "現在の状態: {state}。",
    additionalConfig: "追加設定",
    tabs: {
      Display: "表示",
      API: "API",
      Audio: "音声",
      Network: "ネットワーク",
      Hotkeys: "ホットキー",
      Cache: "キャッシュ",
      Notifications: "通知",
      Debug: "デバッグ",
      About: "情報"
    },
    fields: {
      DISPLAY_LANGUAGE: "表示言語",
      API_ENDPOINT: "API エンドポイント",
      TOKEN: "トークン",
      MODEL: "モデル",
      LANGUAGE: "言語",
      PROMPT: "プロンプト",
      TEXT_PATH: "テキストパス",
      ExtraConfig: "追加設定",
      CHANNELS: "チャンネル",
      SAMPLING_RATE: "サンプリングレート",
      SAMPLING_RATE_DEPTH: "サンプル深度",
      BIT_RATE: "ビットレート",
      CODECS: "コーデック",
      CONTAINER: "コンテナ",
      REQUEST_TIMEOUT: "リクエストタイムアウト",
      MAX_RETRY: "最大リトライ回数",
      RETRY_BASE_DELAY: "リトライ間隔",
      ENABLE_HTTP2: "HTTP/2",
      VERIFY_SSL: "SSL を検証",
      START_KEY: "開始キー",
      PAUSE_KEY: "一時停止キー",
      CANCEL_KEY: "キャンセルキー",
      HOTKEY_HOOK: "低レベルフック",
      CACHE_DIR: "キャッシュディレクトリ",
      KEEP_CACHE: "キャッシュを保持",
      NOTIFICATION: "通知",
      REQUEST_FAILED_NOTIFICATION: "リクエスト失敗プレースホルダー",
      FFMPEG_DEBUG: "FFmpeg デバッグ",
      RECORD_DEBUG: "録音デバッグ",
      HOTKEY_DEBUG: "ホットキーデバッグ",
      UPLOAD_DEBUG: "アップロードデバッグ"
    },
    fieldHelp: {
      TOKEN: "Bearer Authentication Token。他の認証方式はまだサポートしていません。",
      PROMPT: "リクエストフィールド prompt に対応します。API が別の名前を使う場合は、追加設定で設定してください。",
      TEXT_PATH: "API レスポンス JSON から文字起こしテキストを読み取るためのドット区切りパスです。例: results[0].alternatives[0].transcript。OpenAI 互換 API では text を使用します。",
      ExtraConfig: "API リクエストと一緒に送信する追加 JSON リクエストフィールド設定です。例: {\"enable_lid\":true,\"enable_itn\":true}。これらのフィールドはルートのリクエストフィールドにマージされます。{\"prompt\":null} のように組み込みフィールドを null にすると、そのフィールドを削除できます。削除できる組み込みフィールド: model, language, prompt。",
      NOTIFICATION: "Windows システム通知です。有効化は推奨しません。"
    },
    about: {
      Author: "作者",
      Email: "メール",
      License: "ライセンス",
      GitHub: "GitHub"
    }
  },
  fr: {
    appTitle: "STT",
    settings: "Paramètres",
    closeSettings: "Fermer les paramètres",
    startStopRecording: "Démarrer ou arrêter l'enregistrement",
    pauseResume: "Mettre en pause ou reprendre",
    cancelRecording: "Annuler l'enregistrement",
    minimal: "Minimal",
    idle: "Inactif",
    cancel: "Annuler",
    save: "Enregistrer",
    saving: "Enregistrement...",
    saved: "Enregistré",
    quitTitle: "Quitter STT ?",
    quit: "Quitter",
    currentState: "État actuel : {state}.",
    additionalConfig: "Configuration supplémentaire",
    tabs: {
      Display: "Affichage",
      API: "API",
      Audio: "Audio",
      Network: "Réseau",
      Hotkeys: "Raccourcis",
      Cache: "Cache",
      Notifications: "Notifications",
      Debug: "Débogage",
      About: "À propos"
    },
    fields: {
      DISPLAY_LANGUAGE: "Langue d'affichage",
      API_ENDPOINT: "Point de terminaison API",
      TOKEN: "Jeton",
      MODEL: "Modèle",
      LANGUAGE: "Langue",
      PROMPT: "Invite",
      TEXT_PATH: "Chemin du texte",
      ExtraConfig: "Configuration supplémentaire",
      CHANNELS: "Canaux",
      SAMPLING_RATE: "Fréquence d'échantillonnage",
      SAMPLING_RATE_DEPTH: "Profondeur d'échantillonnage",
      BIT_RATE: "Débit binaire",
      CODECS: "Codec",
      CONTAINER: "Conteneur",
      REQUEST_TIMEOUT: "Délai de requête",
      MAX_RETRY: "Nombre max. de tentatives",
      RETRY_BASE_DELAY: "Délai de nouvelle tentative",
      ENABLE_HTTP2: "HTTP/2",
      VERIFY_SSL: "Vérifier SSL",
      START_KEY: "Touche de démarrage",
      PAUSE_KEY: "Touche de pause",
      CANCEL_KEY: "Touche d'annulation",
      HOTKEY_HOOK: "Hook bas niveau",
      CACHE_DIR: "Dossier du cache",
      KEEP_CACHE: "Conserver le cache",
      NOTIFICATION: "Notification",
      REQUEST_FAILED_NOTIFICATION: "Espace réservé en cas d'échec",
      FFMPEG_DEBUG: "Débogage FFmpeg",
      RECORD_DEBUG: "Débogage de l'enregistrement",
      HOTKEY_DEBUG: "Débogage des raccourcis",
      UPLOAD_DEBUG: "Débogage de l'envoi"
    },
    fieldHelp: {
      TOKEN: "Bearer Authentication Token. Les autres types d'authentification ne sont pas encore pris en charge.",
      PROMPT: "Correspond au champ de requête prompt. Si une API utilise un autre nom, configurez-le dans la configuration supplémentaire.",
      TEXT_PATH: "Chemin JSON à points utilisé pour lire le texte transcrit dans la réponse de l'API. Exemple : results[0].alternatives[0].transcript. Utilisez text pour les API compatibles OpenAI.",
      ExtraConfig: "Champs de requête JSON supplémentaires à envoyer avec la requête API. Exemple : {\"enable_lid\":true,\"enable_itn\":true}. Ces champs sont fusionnés dans les champs racine de la requête. Définir un champ intégré sur null, comme {\"prompt\":null}, le supprime. Champs intégrés supprimables : model, language, prompt.",
      NOTIFICATION: "Notification système Windows. Non recommandé."
    },
    about: {
      Author: "Auteur",
      Email: "E-mail",
      License: "Licence",
      GitHub: "GitHub"
    }
  }
};

const groups = [
  {
    name: "Display",
    fields: []
  },
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
    name: "About",
    fields: []
  }
];

const aboutInfo = {
  title: "STT for Windows",
  rows: [
    ["Author", "Joey Kot"],
    ["Email", "joey.kot.x@gmail.com"],
    ["License", "GPL-3.0"],
    ["GitHub", "github.com/Joey-Kot/STT-for-Windows"]
  ]
};

const fieldMeta = {
  API_ENDPOINT: { type: "url" },
  TOKEN: { type: "password" },
  MODEL: { type: "text" },
  LANGUAGE: { type: "text" },
  PROMPT: { type: "textarea" },
  TEXT_PATH: { type: "text" },
  ExtraConfig: { type: "textarea" },
  CHANNELS: { type: "number" },
  SAMPLING_RATE: { type: "number" },
  SAMPLING_RATE_DEPTH: { type: "number" },
  BIT_RATE: { type: "number" },
  CODECS: { type: "text" },
  CONTAINER: { type: "text" },
  REQUEST_TIMEOUT: { type: "number" },
  MAX_RETRY: { type: "number" },
  RETRY_BASE_DELAY: { type: "number", step: "0.1" },
  ENABLE_HTTP2: { type: "checkbox" },
  VERIFY_SSL: { type: "checkbox" },
  START_KEY: { type: "text" },
  PAUSE_KEY: { type: "text" },
  CANCEL_KEY: { type: "text" },
  HOTKEY_HOOK: { type: "checkbox" },
  CACHE_DIR: { type: "text" },
  KEEP_CACHE: { type: "checkbox" },
  NOTIFICATION: { type: "checkbox" },
  REQUEST_FAILED_NOTIFICATION: { type: "checkbox" },
  FFMPEG_DEBUG: { type: "checkbox" },
  RECORD_DEBUG: { type: "checkbox" },
  HOTKEY_DEBUG: { type: "checkbox" },
  UPLOAD_DEBUG: { type: "checkbox" }
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
  activeGroup: "Display",
  displayLanguage: loadDisplayLanguage()
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
  quitTitle: document.querySelector("#quitDialog h2"),
  keepRunningBtn: document.getElementById("keepRunningBtn"),
  confirmQuitBtn: document.getElementById("confirmQuitBtn")
};

function loadDisplayLanguage() {
  try {
    const stored = localStorage.getItem(DISPLAY_LANGUAGE_KEY);
    return translations[stored] ? stored : DEFAULT_DISPLAY_LANGUAGE;
  } catch {
    return DEFAULT_DISPLAY_LANGUAGE;
  }
}

function saveDisplayLanguage(language) {
  try {
    localStorage.setItem(DISPLAY_LANGUAGE_KEY, language);
  } catch {
    // Display language is GUI-only preference; ignore unavailable storage.
  }
}

function t(path, replacements = {}) {
  const parts = path.split(".");
  let value = translations[state.displayLanguage] || translations[DEFAULT_DISPLAY_LANGUAGE];
  let fallback = translations[DEFAULT_DISPLAY_LANGUAGE];

  for (const part of parts) {
    value = value?.[part];
    fallback = fallback?.[part];
  }

  const text = typeof value === "string" ? value : fallback || path;
  return Object.entries(replacements).reduce(
    (result, [key, replacement]) => result.replaceAll(`{${key}}`, replacement),
    text
  );
}

function optionalTranslation(path) {
  const parts = path.split(".");
  const locales = [translations[state.displayLanguage], translations[DEFAULT_DISPLAY_LANGUAGE]];

  for (const locale of locales) {
    let value = locale;
    for (const part of parts) {
      value = value?.[part];
    }
    if (typeof value === "string") {
      return value;
    }
  }

  return "";
}

function fieldLabel(key, fallback = key) {
  const text = t(`fields.${key}`);
  return text === `fields.${key}` ? fallback : text;
}

function setButtonText(button, text) {
  button.textContent = text;
}

function setAccessibleLabel(element, text) {
  element.title = text;
  element.setAttribute("aria-label", text);
}

function applyStaticTranslations() {
  document.documentElement.lang = state.displayLanguage;
  document.title = t("appTitle");
  document.querySelector(".settingsHeader h1").textContent = t("settings");
  setAccessibleLabel(el.recordBtn, t("startStopRecording"));
  setAccessibleLabel(el.pauseBtn, t("pauseResume"));
  setAccessibleLabel(el.cancelBtn, t("cancelRecording"));
  setAccessibleLabel(el.settingsBtn, t("settings"));
  setAccessibleLabel(el.minimalBtn, t("minimal"));
  setAccessibleLabel(el.closeSettingsBtn, t("closeSettings"));
  setButtonText(el.cancelSettingsBtn, t("cancel"));
  setButtonText(el.saveSettingsBtn, t("save"));
  setButtonText(el.keepRunningBtn, t("cancel"));
  setButtonText(el.confirmQuitBtn, t("quit"));
  el.quitTitle.textContent = t("quitTitle");
}

function applyTranslations() {
  applyStaticTranslations();
  setRuntime();
  renderTabs();
  if (!el.settingsView.classList.contains("hidden")) {
    renderFields();
  }
}

function setRuntime(next) {
  state.runtime = next || state.runtime;
  const runtimeState = state.runtime.state || "Idle";
  el.panel.dataset.state = runtimeState;
  el.recordBtn.dataset.mode = runtimeState.toLowerCase();
  const runtimeMessage = state.runtime.message === "Idle" ? t("idle") : state.runtime.message;
  el.status.textContent = state.runtime.error || runtimeMessage || (runtimeState === "Idle" ? t("idle") : runtimeState);
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
    button.textContent = t(`tabs.${group.name}`);
    button.dataset.active = String(group.name === state.activeGroup);
    button.addEventListener("click", () => {
      state.activeGroup = group.name;
      renderTabs();
      renderFields();
    });
    el.tabs.appendChild(button);
  }
}

function getUngroupedConfigFields() {
  return Object.keys(state.config).filter((key) =>
    !groups.some((item) => item.name !== "About" && item.name !== "Display" && item.fields.includes(key))
  );
}

function renderAbout() {
  const section = document.createElement("section");
  section.className = "about";

  const title = document.createElement("h2");
  title.textContent = aboutInfo.title;
  section.appendChild(title);

  const details = document.createElement("dl");
  details.className = "aboutDetails";
  for (const [label, value] of aboutInfo.rows) {
    const term = document.createElement("dt");
    term.textContent = `${t(`about.${label}`)}:`;
    details.appendChild(term);

    const description = document.createElement("dd");
    if (label === "Email") {
      const link = document.createElement("a");
      link.href = `mailto:${value}`;
      link.textContent = value;
      description.appendChild(link);
    } else if (label === "GitHub") {
      const link = document.createElement("a");
      link.href = `https://${value}`;
      link.target = "_blank";
      link.rel = "noreferrer";
      link.textContent = value;
      description.appendChild(link);
    } else {
      description.textContent = value;
    }
    details.appendChild(description);
  }
  section.appendChild(details);
  el.configForm.appendChild(section);
}

function renderDisplaySettings() {
  const row = document.createElement("label");
  row.className = "field";
  row.htmlFor = "field-DISPLAY_LANGUAGE";

  const label = document.createElement("span");
  label.textContent = t("fields.DISPLAY_LANGUAGE");
  row.appendChild(label);

  const select = document.createElement("select");
  select.id = "field-DISPLAY_LANGUAGE";

  for (const language of displayLanguages) {
    const option = document.createElement("option");
    option.value = language.value;
    option.textContent = language.label;
    select.appendChild(option);
  }
  select.value = state.displayLanguage;

  select.addEventListener("change", () => {
    state.displayLanguage = translations[select.value] ? select.value : DEFAULT_DISPLAY_LANGUAGE;
    saveDisplayLanguage(state.displayLanguage);
    applyTranslations();
  });

  row.appendChild(select);
  el.configForm.appendChild(row);
}

function renderConfigField(key) {
  const meta = fieldMeta[key] || { label: key, type: typeof state.config[key] === "boolean" ? "checkbox" : "text" };
  const row = document.createElement("label");
  row.className = meta.type === "checkbox" ? "field checkField" : "field";
  row.htmlFor = `field-${key}`;

  const label = document.createElement("span");
  label.textContent = fieldLabel(key, meta.label);
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

  const control = document.createElement("div");
  control.className = "fieldControl";
  control.appendChild(input);

  const helpText = optionalTranslation(`fieldHelp.${key}`);
  if (helpText) {
    const help = document.createElement("p");
    help.className = "fieldHelp";
    help.textContent = helpText;
    control.appendChild(help);
  }

  row.appendChild(control);
  el.configForm.appendChild(row);
}

function renderFields() {
  el.configForm.innerHTML = "";
  const group = groups.find((item) => item.name === state.activeGroup);
  if (group.name === "Display") {
    renderDisplaySettings();
    return;
  }

  const fields = group.name === "About" ? getUngroupedConfigFields() : group.fields;

  if (group.name === "About") {
    renderAbout();

    if (fields.length > 0) {
      const divider = document.createElement("div");
      divider.className = "aboutDivider";
      divider.textContent = t("additionalConfig");
      el.configForm.appendChild(divider);
    }
  }

  for (const key of fields) {
    renderConfigField(key);
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
  state.activeGroup = "Display";
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
  el.saveStatus.textContent = t("saving");
  try {
    const payload = JSON.stringify(state.config, null, 2);
    const next = await SaveConfig(payload);
    setRuntime(next);
    el.saveStatus.textContent = t("saved");
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
  el.quitMessage.textContent = t("currentState", { state: snapshot.state });
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

applyStaticTranslations();
setRuntime(await GetState());
applyUIStyle(await GetUIStyle());
await applyWindowState(await GetWindowState());
