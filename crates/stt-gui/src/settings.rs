use std::collections::HashMap;
use std::ffi::c_void;
use std::mem::size_of;
use std::sync::Arc;

use stt_core::Config;
use stt_core::runtime::{Event, Runtime};
use windows::Win32::Foundation::{COLORREF, HWND, LPARAM, LRESULT, POINT, RECT, WPARAM};
use windows::Win32::Graphics::Gdi::{
    BeginPaint, CLEARTYPE_QUALITY, CLIP_DEFAULT_PRECIS, CreateFontW, CreatePen, CreateSolidBrush,
    DC_BRUSH, DC_PEN, DEFAULT_CHARSET, DEFAULT_PITCH, DT_CENTER, DT_END_ELLIPSIS, DT_LEFT,
    DT_NOPREFIX, DT_SINGLELINE, DT_VCENTER, DeleteObject, DrawTextW, EndPaint, FF_DONTCARE,
    FW_BOLD, FW_NORMAL, FillRect, GetStockObject, HBRUSH, HDC, HFONT, HGDIOBJ, InvalidateRect,
    LineTo, MoveToEx, OUT_DEFAULT_PRECIS, PAINTSTRUCT, PS_SOLID, RoundRect, ScreenToClient,
    SelectObject, SetBkColor, SetBkMode, SetDCBrushColor, SetDCPenColor, SetTextColor, TRANSPARENT,
};
use windows::Win32::System::LibraryLoader::GetModuleHandleW;
use windows::Win32::UI::Controls::{
    DRAWITEMSTRUCT, EM_SETMARGINS, ODS_DISABLED, ODS_FOCUS, ODS_HOTLIGHT, ODS_SELECTED,
    SetWindowTheme,
};
use windows::Win32::UI::Input::KeyboardAndMouse::{EnableWindow, GetFocus};
use windows::Win32::UI::WindowsAndMessaging::{
    BN_CLICKED, BS_OWNERDRAW, CREATESTRUCTW, CS_HREDRAW, CS_VREDRAW, CreateWindowExW, DI_NORMAL,
    DefWindowProcW, DestroyWindow, DrawIconEx, EC_LEFTMARGIN, EC_RIGHTMARGIN, EN_KILLFOCUS,
    EN_SETFOCUS, ES_AUTOHSCROLL, ES_AUTOVSCROLL, ES_MULTILINE, ES_PASSWORD, ES_WANTRETURN,
    GWLP_USERDATA, GetClientRect, GetMessagePos, GetWindowLongPtrW, GetWindowTextLengthW,
    GetWindowTextW, HCURSOR, HMENU, HTCAPTION, HTCLIENT, HWND_TOP, IDC_ARROW, LoadCursorW,
    MB_ICONERROR, MB_OK, MessageBoxW, PostMessageW, RegisterClassExW, SW_HIDE, SW_SHOW,
    SWP_NOACTIVATE, SWP_NOMOVE, SWP_NOSIZE, SendMessageW, SetWindowLongPtrW, SetWindowPos,
    SetWindowTextW, ShowWindow, WINDOW_EX_STYLE, WINDOW_STYLE, WM_CLOSE, WM_COMMAND, WM_CREATE,
    WM_CTLCOLORBTN, WM_CTLCOLORDLG, WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX, WM_CTLCOLORSTATIC,
    WM_DESTROY, WM_DPICHANGED, WM_DRAWITEM, WM_ERASEBKGND, WM_LBUTTONDOWN, WM_NCCREATE,
    WM_NCHITTEST, WM_PAINT, WM_SETFONT, WNDCLASSEXW, WS_CHILD, WS_CLIPCHILDREN, WS_EX_TOOLWINDOW,
    WS_POPUP, WS_TABSTOP, WS_VISIBLE,
};
use windows::core::{PCWSTR, w};

use crate::i18n::Language;
use crate::platform;
use crate::resources;

const ID_SAVE: usize = 0x6101;
const ID_CANCEL: usize = 0x6102;
const ID_LANGUAGE: usize = 0x6103;
const ID_CLOSE: usize = 0x6104;
const ID_PAGE_BASE: usize = 0x6110;
const ID_LANGUAGE_ITEM_BASE: usize = 0x6180;
const ID_FIELD_BASE: usize = 0x6200;
const WM_SAVE_RESULT: u32 = windows::Win32::UI::WindowsAndMessaging::WM_APP + 21;
pub const WM_LANGUAGE_CHANGED: u32 = windows::Win32::UI::WindowsAndMessaging::WM_APP + 22;

const WINDOW_WIDTH: i32 = 760;
const WINDOW_HEIGHT: i32 = 620;
const HEADER_HEIGHT: i32 = 72;
const FOOTER_HEIGHT: i32 = 58;
const SIDEBAR_WIDTH: i32 = 170;
const CONTENT_LEFT: i32 = 196;
const LABEL_WIDTH: i32 = 148;
const EDIT_LEFT: i32 = 354;
const EDIT_WIDTH: i32 = 378;
const FIELD_TOP: i32 = 94;
const FIELD_HEIGHT: i32 = 34;
const SS_CENTERIMAGE_STYLE: u32 = 0x0000_0200;

const GROUPS: &[(&str, &str)] = &[
    ("Display", "display"),
    ("API", "API"),
    ("Audio", "audio"),
    ("Network", "network"),
    ("Hotkeys", "hotkeys"),
    ("Cache", "cache"),
    ("Debug", "debug"),
    ("About", "about"),
];

#[derive(Clone, Copy)]
enum FieldKind {
    Text,
    Password,
    Integer,
    Float,
    Boolean,
    Multiline,
}

struct FieldSpec {
    key: &'static str,
    label: &'static str,
    group: &'static str,
    kind: FieldKind,
}

const FIELDS: &[FieldSpec] = &[
    FieldSpec {
        key: "API_ENDPOINT",
        label: "API endpoint",
        group: "API",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "TOKEN",
        label: "Token",
        group: "API",
        kind: FieldKind::Password,
    },
    FieldSpec {
        key: "MODEL",
        label: "Model",
        group: "API",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "LANGUAGE",
        label: "Language",
        group: "API",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "PROMPT",
        label: "Prompt",
        group: "API",
        kind: FieldKind::Multiline,
    },
    FieldSpec {
        key: "TEXT_PATH",
        label: "Text path",
        group: "API",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "ExtraConfig",
        label: "Extra config",
        group: "API",
        kind: FieldKind::Multiline,
    },
    FieldSpec {
        key: "CHANNELS",
        label: "Channels",
        group: "Audio",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "SAMPLING_RATE",
        label: "Sampling rate",
        group: "Audio",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "SAMPLING_RATE_DEPTH",
        label: "Sample depth",
        group: "Audio",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "BIT_RATE",
        label: "Bit rate",
        group: "Audio",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "CODECS",
        label: "Codec",
        group: "Audio",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "CONTAINER",
        label: "Container",
        group: "Audio",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "REQUEST_TIMEOUT",
        label: "Request timeout",
        group: "Network",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "MAX_RETRY",
        label: "Max retry",
        group: "Network",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "RETRY_BASE_DELAY",
        label: "Retry delay",
        group: "Network",
        kind: FieldKind::Float,
    },
    FieldSpec {
        key: "ENABLE_HTTP2",
        label: "HTTP/2",
        group: "Network",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "VERIFY_SSL",
        label: "Verify SSL",
        group: "Network",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "START_KEY",
        label: "Start key",
        group: "Hotkeys",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "PAUSE_KEY",
        label: "Pause key",
        group: "Hotkeys",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "CANCEL_KEY",
        label: "Cancel key",
        group: "Hotkeys",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "HOTKEY_HOOK",
        label: "Low-level hook",
        group: "Hotkeys",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "CLIPBOARD_WRITE_DELAY",
        label: "Paste delay (ms)",
        group: "Hotkeys",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "CLIPBOARD_RESTORE_DELAY",
        label: "Restore delay (ms)",
        group: "Hotkeys",
        kind: FieldKind::Integer,
    },
    FieldSpec {
        key: "CACHE_DIR",
        label: "Cache dir",
        group: "Cache",
        kind: FieldKind::Text,
    },
    FieldSpec {
        key: "KEEP_CACHE",
        label: "Keep cache",
        group: "Cache",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "REQUEST_FAILED_NOTIFICATION",
        label: "Request failed placeholder",
        group: "Cache",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "FFMPEG_DEBUG",
        label: "FFmpeg debug",
        group: "Debug",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "RECORD_DEBUG",
        label: "Record debug",
        group: "Debug",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "HOTKEY_DEBUG",
        label: "Hotkey debug",
        group: "Debug",
        kind: FieldKind::Boolean,
    },
    FieldSpec {
        key: "UPLOAD_DEBUG",
        label: "Upload debug",
        group: "Debug",
        kind: FieldKind::Boolean,
    },
];

struct SettingsState {
    hwnd: HWND,
    owner: HWND,
    runtime: Arc<Runtime>,
    controls: HashMap<&'static str, HWND>,
    config_path: std::path::PathBuf,
    saving: bool,
    language: Language,
    language_control: HWND,
    language_items: Vec<HWND>,
    language_open: bool,
    boolean_values: HashMap<&'static str, bool>,
    boolean_ids: HashMap<usize, &'static str>,
    input_frames: Vec<InputFrame>,
    control_groups: HashMap<usize, &'static str>,
    localized_controls: Vec<(HWND, &'static str)>,
    page_buttons: Vec<HWND>,
    active_group: usize,
    dpi: u32,
    background_brush: HBRUSH,
    input_brush: HBRUSH,
    font: HFONT,
    title_font: HFONT,
    small_font: HFONT,
}

#[derive(Clone, Copy)]
struct InputFrame {
    rect: RECT,
    group: &'static str,
    control: HWND,
}

pub struct SettingsWindow {
    hwnd: HWND,
}

impl SettingsWindow {
    pub fn open(owner: HWND, runtime: Arc<Runtime>, language: Language) -> Result<Self, String> {
        let instance = unsafe { GetModuleHandleW(None).map_err(|error| error.to_string())? };
        let cursor: HCURSOR =
            unsafe { LoadCursorW(None, IDC_ARROW).map_err(|error| error.to_string())? };
        let icon = resources::load_app_icon().map_err(|error| error.to_string())?;
        let class = WNDCLASSEXW {
            cbSize: size_of::<WNDCLASSEXW>() as u32,
            style: CS_HREDRAW | CS_VREDRAW,
            lpfnWndProc: Some(settings_proc),
            hInstance: instance.into(),
            hIcon: icon,
            hCursor: cursor,
            lpszClassName: w!("STTRustSettingsWindow"),
            hIconSm: icon,
            ..Default::default()
        };
        unsafe {
            RegisterClassExW(&class);
        }

        let dpi = platform::window_dpi(owner);
        let state = Box::new(SettingsState {
            hwnd: HWND::default(),
            owner,
            runtime,
            controls: HashMap::new(),
            config_path: platform::config_path()?,
            saving: false,
            language,
            language_control: HWND::default(),
            language_items: Vec::new(),
            language_open: false,
            boolean_values: HashMap::new(),
            boolean_ids: HashMap::new(),
            input_frames: Vec::new(),
            control_groups: HashMap::new(),
            localized_controls: Vec::new(),
            page_buttons: Vec::new(),
            active_group: 0,
            dpi,
            background_brush: unsafe { CreateSolidBrush(rgb(16, 22, 25)) },
            input_brush: unsafe { CreateSolidBrush(rgb(26, 35, 39)) },
            font: create_font(dpi, 14, false),
            title_font: create_font(dpi, 20, true),
            small_font: create_font(dpi, 11, false),
        });
        let pointer = Box::into_raw(state);
        let hwnd = unsafe {
            CreateWindowExW(
                WS_EX_TOOLWINDOW,
                w!("STTRustSettingsWindow"),
                PCWSTR(wide(language.text("settings")).as_ptr()),
                WS_POPUP | WS_VISIBLE | WS_CLIPCHILDREN,
                windows::Win32::UI::WindowsAndMessaging::CW_USEDEFAULT,
                windows::Win32::UI::WindowsAndMessaging::CW_USEDEFAULT,
                platform::scale(WINDOW_WIDTH, dpi),
                platform::scale(WINDOW_HEIGHT, dpi),
                Some(owner),
                None,
                Some(instance.into()),
                Some(pointer.cast::<c_void>()),
            )
        }
        .map_err(|error| {
            unsafe { drop(Box::from_raw(pointer)) };
            error.to_string()
        })?;
        unsafe {
            platform::apply_dark_mode(hwnd);
            platform::apply_corner_preference(hwnd, platform::supports_rounded_corners());
            let _ = ShowWindow(hwnd, SW_SHOW);
        }
        Ok(Self { hwnd })
    }

    pub fn hwnd(&self) -> HWND {
        self.hwnd
    }
}

unsafe extern "system" fn settings_proc(
    hwnd: HWND,
    message: u32,
    wparam: WPARAM,
    lparam: LPARAM,
) -> LRESULT {
    if message == WM_NCCREATE {
        let create = unsafe { &*(lparam.0 as *const CREATESTRUCTW) };
        let state = create.lpCreateParams as *mut SettingsState;
        unsafe {
            (*state).hwnd = hwnd;
            SetWindowLongPtrW(hwnd, GWLP_USERDATA, state as isize);
        }
    }
    let pointer = unsafe { GetWindowLongPtrW(hwnd, GWLP_USERDATA) } as *mut SettingsState;
    if pointer.is_null() {
        return unsafe { DefWindowProcW(hwnd, message, wparam, lparam) };
    }
    let state = unsafe { &mut *pointer };
    match message {
        WM_CREATE => {
            if let Err(error) = create_controls(state) {
                show_error(hwnd, &error);
            }
            LRESULT(0)
        }
        WM_NCHITTEST => {
            let packed = unsafe { GetMessagePos() };
            let mut point = POINT {
                x: (packed as u16) as i16 as i32,
                y: ((packed >> 16) as u16) as i16 as i32,
            };
            unsafe {
                let _ = ScreenToClient(hwnd, &mut point);
            }
            let logical_y = platform::unscale(point.y, state.dpi);
            let logical_x = platform::unscale(point.x, state.dpi);
            if logical_y < HEADER_HEIGHT && logical_x < WINDOW_WIDTH - 64 {
                LRESULT(HTCAPTION as isize)
            } else {
                LRESULT(HTCLIENT as isize)
            }
        }
        WM_COMMAND => {
            let id = wparam.0 & 0xffff;
            let notification = ((wparam.0 >> 16) & 0xffff) as u32;
            if id >= ID_PAGE_BASE && id < ID_PAGE_BASE + GROUPS.len() && notification == BN_CLICKED
            {
                set_language_dropdown(state, false);
                state.active_group = id - ID_PAGE_BASE;
                update_page_visibility(state);
                for button in &state.page_buttons {
                    unsafe {
                        let _ = InvalidateRect(Some(*button), None, true);
                    }
                }
                unsafe {
                    let _ = InvalidateRect(Some(hwnd), None, true);
                }
            } else if id == ID_LANGUAGE && notification == BN_CLICKED {
                set_language_dropdown(state, !state.language_open);
            } else if id >= ID_LANGUAGE_ITEM_BASE
                && id < ID_LANGUAGE_ITEM_BASE + Language::ALL.len()
                && notification == BN_CLICKED
            {
                select_language(state, id - ID_LANGUAGE_ITEM_BASE);
            } else if let Some(key) = state.boolean_ids.get(&id).copied()
                && notification == BN_CLICKED
            {
                let value = state.boolean_values.entry(key).or_default();
                *value = !*value;
                if let Some(control) = state.controls.get(key) {
                    unsafe {
                        let _ = InvalidateRect(Some(*control), None, true);
                    }
                }
            } else if id >= ID_FIELD_BASE
                && id < ID_FIELD_BASE + FIELDS.len()
                && (notification == EN_SETFOCUS || notification == EN_KILLFOCUS)
            {
                unsafe {
                    let _ = InvalidateRect(Some(hwnd), None, true);
                }
            } else {
                match id {
                    ID_SAVE => save(state),
                    ID_CANCEL | ID_CLOSE => unsafe {
                        let _ = DestroyWindow(hwnd);
                    },
                    _ => {}
                }
            }
            LRESULT(0)
        }
        WM_LBUTTONDOWN => {
            if state.language_open {
                set_language_dropdown(state, false);
            }
            unsafe { DefWindowProcW(hwnd, message, wparam, lparam) }
        }
        WM_PAINT => {
            paint_window(state);
            LRESULT(0)
        }
        WM_ERASEBKGND => LRESULT(1),
        WM_DRAWITEM => {
            let item = unsafe { &*(lparam.0 as *const DRAWITEMSTRUCT) };
            draw_owner_button(state, item);
            LRESULT(1)
        }
        WM_CTLCOLORSTATIC | WM_CTLCOLORBTN => {
            let hdc = HDC(wparam.0 as *mut c_void);
            unsafe {
                SetBkMode(hdc, TRANSPARENT);
                SetTextColor(hdc, rgb(188, 202, 205));
            }
            LRESULT(state.background_brush.0 as isize)
        }
        WM_CTLCOLOREDIT | WM_CTLCOLORLISTBOX => {
            let hdc = HDC(wparam.0 as *mut c_void);
            unsafe {
                SetBkColor(hdc, rgb(26, 35, 39));
                SetTextColor(hdc, rgb(235, 242, 243));
            }
            LRESULT(state.input_brush.0 as isize)
        }
        WM_CTLCOLORDLG => LRESULT(state.background_brush.0 as isize),
        WM_DPICHANGED => {
            state.dpi = ((wparam.0 >> 16) & 0xffff) as u32;
            let suggested = unsafe { &*(lparam.0 as *const RECT) };
            unsafe {
                let _ = SetWindowPos(
                    hwnd,
                    None,
                    suggested.left,
                    suggested.top,
                    suggested.right - suggested.left,
                    suggested.bottom - suggested.top,
                    SWP_NOACTIVATE,
                );
            }
            LRESULT(0)
        }
        WM_SAVE_RESULT => {
            let result = unsafe { Box::from_raw(lparam.0 as *mut Result<Event, String>) };
            state.saving = false;
            enable_controls(state, true);
            match *result {
                Ok(_) => unsafe {
                    let _ = DestroyWindow(hwnd);
                },
                Err(error) => show_error(hwnd, &error),
            }
            LRESULT(0)
        }
        WM_CLOSE => {
            if !state.saving {
                unsafe {
                    let _ = DestroyWindow(hwnd);
                }
            }
            LRESULT(0)
        }
        WM_DESTROY => {
            unsafe {
                let _ = DeleteObject(HGDIOBJ(state.background_brush.0));
                let _ = DeleteObject(HGDIOBJ(state.input_brush.0));
                let _ = DeleteObject(HGDIOBJ(state.font.0));
                let _ = DeleteObject(HGDIOBJ(state.title_font.0));
                let _ = DeleteObject(HGDIOBJ(state.small_font.0));
                SetWindowLongPtrW(hwnd, GWLP_USERDATA, 0);
                drop(Box::from_raw(pointer));
            }
            LRESULT(0)
        }
        _ => unsafe { DefWindowProcW(hwnd, message, wparam, lparam) },
    }
}

fn create_controls(state: &mut SettingsState) -> Result<(), String> {
    let config = state.runtime.config();
    let values = serde_json::to_value(&config).map_err(|error| error.to_string())?;
    let object = values
        .as_object()
        .ok_or("config serialization was not an object")?;
    let instance = unsafe { GetModuleHandleW(None).map_err(|error| error.to_string())? };

    for (index, (group, key)) in GROUPS.iter().enumerate() {
        let button = create_child(
            state,
            w!("BUTTON"),
            if *key == "API" {
                "API"
            } else {
                state.language.text(key)
            },
            WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
            12,
            84 + index as i32 * 36,
            146,
            32,
            ID_PAGE_BASE + index,
            instance,
        )?;
        state.page_buttons.push(button);
        state.localized_controls.push((button, key));
        let _ = group;
    }

    let close = create_child(
        state,
        w!("BUTTON"),
        "×",
        WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
        706,
        17,
        38,
        38,
        ID_CLOSE,
        instance,
    )?;
    state.controls.insert("__close", close);

    let display_label = create_label(
        state,
        state.language.text("display_language"),
        CONTENT_LEFT,
        96,
        LABEL_WIDTH,
        FIELD_HEIGHT,
        instance,
    )?;
    state
        .control_groups
        .insert(display_label.0 as usize, "Display");
    state
        .localized_controls
        .push((display_label, "display_language"));
    state.language_control = create_child(
        state,
        w!("BUTTON"),
        state.language.native_name(),
        WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
        EDIT_LEFT,
        96,
        EDIT_WIDTH,
        FIELD_HEIGHT,
        ID_LANGUAGE,
        instance,
    )?;
    state
        .control_groups
        .insert(state.language_control.0 as usize, "Display");

    let mut group_y: HashMap<&'static str, i32> = HashMap::new();
    for (index, field) in FIELDS.iter().enumerate() {
        let y = *group_y.entry(field.group).or_insert(FIELD_TOP);
        let label = create_label(
            state,
            state.language.text(field.key),
            CONTENT_LEFT,
            y,
            LABEL_WIDTH,
            FIELD_HEIGHT,
            instance,
        )?;
        state.control_groups.insert(label.0 as usize, field.group);
        state.localized_controls.push((label, field.key));

        let value = object.get(field.key).cloned().unwrap_or_default();
        let control = match field.kind {
            FieldKind::Boolean => {
                let id = ID_FIELD_BASE + index;
                let hwnd = create_child(
                    state,
                    w!("BUTTON"),
                    "",
                    WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
                    EDIT_LEFT,
                    y + 5,
                    24,
                    24,
                    id,
                    instance,
                )?;
                state
                    .boolean_values
                    .insert(field.key, value.as_bool().unwrap_or(false));
                state.boolean_ids.insert(id, field.key);
                hwnd
            }
            kind => {
                let text = match value {
                    serde_json::Value::String(text) => text,
                    other => other.to_string(),
                };
                let multiline = matches!(kind, FieldKind::Multiline);
                let frame_height = if multiline { 68 } else { FIELD_HEIGHT };
                let mut style = WS_CHILD | WS_VISIBLE | WS_TABSTOP;
                style |= WINDOW_STYLE(if multiline {
                    (ES_MULTILINE | ES_AUTOVSCROLL | ES_WANTRETURN) as u32
                } else {
                    ES_AUTOHSCROLL as u32
                });
                if matches!(kind, FieldKind::Password) {
                    style |= WINDOW_STYLE(ES_PASSWORD as u32);
                }
                let hwnd = create_child(
                    state,
                    w!("EDIT"),
                    &text,
                    style,
                    EDIT_LEFT + 3,
                    y + if multiline { 5 } else { 7 },
                    EDIT_WIDTH - 6,
                    if multiline { frame_height - 10 } else { 20 },
                    ID_FIELD_BASE + index,
                    instance,
                )?;
                apply_dark_theme(hwnd);
                unsafe {
                    let margin = platform::scale(7, state.dpi) as u32;
                    SendMessageW(
                        hwnd,
                        EM_SETMARGINS,
                        Some(WPARAM((EC_LEFTMARGIN | EC_RIGHTMARGIN) as usize)),
                        Some(LPARAM((margin | (margin << 16)) as isize)),
                    );
                }
                state.input_frames.push(InputFrame {
                    rect: RECT {
                        left: EDIT_LEFT,
                        top: y,
                        right: EDIT_LEFT + EDIT_WIDTH,
                        bottom: y + frame_height,
                    },
                    group: field.group,
                    control: hwnd,
                });
                hwnd
            }
        };
        state.controls.insert(field.key, control);
        state.control_groups.insert(control.0 as usize, field.group);
        group_y.insert(
            field.group,
            y + if matches!(field.kind, FieldKind::Multiline) {
                80
            } else {
                42
            },
        );
    }

    let cancel = create_child(
        state,
        w!("BUTTON"),
        state.language.text("cancel"),
        WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
        552,
        574,
        88,
        34,
        ID_CANCEL,
        instance,
    )?;
    let save = create_child(
        state,
        w!("BUTTON"),
        state.language.text("save"),
        WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
        650,
        574,
        94,
        34,
        ID_SAVE,
        instance,
    )?;
    state.controls.insert("__cancel", cancel);
    state.controls.insert("__save", save);
    state.localized_controls.push((cancel, "cancel"));
    state.localized_controls.push((save, "save"));

    for (index, language) in Language::ALL.iter().enumerate() {
        let item = create_child(
            state,
            w!("BUTTON"),
            language.native_name(),
            WINDOW_STYLE(WS_CHILD.0 | WS_TABSTOP.0 | BS_OWNERDRAW as u32),
            EDIT_LEFT,
            136 + index as i32 * 36,
            EDIT_WIDTH,
            32,
            ID_LANGUAGE_ITEM_BASE + index,
            instance,
        )?;
        unsafe {
            let _ = ShowWindow(item, SW_HIDE);
        }
        state.language_items.push(item);
    }

    update_page_visibility(state);
    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn create_child(
    state: &SettingsState,
    class: PCWSTR,
    text: &str,
    style: WINDOW_STYLE,
    x: i32,
    y: i32,
    width: i32,
    height: i32,
    id: usize,
    instance: windows::Win32::Foundation::HMODULE,
) -> Result<HWND, String> {
    let text = wide(text);
    let hwnd = unsafe {
        CreateWindowExW(
            WINDOW_EX_STYLE::default(),
            class,
            PCWSTR(text.as_ptr()),
            style,
            platform::scale(x, state.dpi),
            platform::scale(y, state.dpi),
            platform::scale(width, state.dpi),
            platform::scale(height, state.dpi),
            Some(state.hwnd),
            (id != 0).then_some(HMENU(id as *mut c_void)),
            Some(instance.into()),
            None,
        )
    }
    .map_err(|error| error.to_string())?;
    set_font(hwnd, state.font);
    Ok(hwnd)
}

fn create_label(
    state: &SettingsState,
    text: &str,
    x: i32,
    y: i32,
    width: i32,
    height: i32,
    instance: windows::Win32::Foundation::HMODULE,
) -> Result<HWND, String> {
    create_child(
        state,
        w!("STATIC"),
        text,
        WINDOW_STYLE(WS_CHILD.0 | WS_VISIBLE.0 | SS_CENTERIMAGE_STYLE),
        x,
        y,
        width,
        height,
        0,
        instance,
    )
}

fn apply_dark_theme(hwnd: HWND) {
    unsafe {
        let _ = SetWindowTheme(hwnd, w!("DarkMode_Explorer"), w!(""));
    }
}

fn paint_window(state: &SettingsState) {
    let mut paint = PAINTSTRUCT::default();
    let hdc = unsafe { BeginPaint(state.hwnd, &mut paint) };
    let scale = |value| platform::scale(value, state.dpi);
    let mut client = RECT::default();
    unsafe {
        let _ = GetClientRect(state.hwnd, &mut client);
        FillRect(hdc, &client, state.background_brush);
        fill_color(
            hdc,
            RECT {
                left: 0,
                top: scale(HEADER_HEIGHT),
                right: scale(SIDEBAR_WIDTH),
                bottom: client.bottom - scale(FOOTER_HEIGHT),
            },
            rgb(12, 18, 21),
        );
        fill_color(
            hdc,
            RECT {
                left: 0,
                top: client.bottom - scale(FOOTER_HEIGHT),
                right: client.right,
                bottom: client.bottom,
            },
            rgb(13, 19, 22),
        );
        line(
            hdc,
            0,
            scale(HEADER_HEIGHT),
            client.right,
            scale(HEADER_HEIGHT),
            rgb(38, 49, 54),
        );
        line(
            hdc,
            scale(SIDEBAR_WIDTH),
            scale(HEADER_HEIGHT),
            scale(SIDEBAR_WIDTH),
            client.bottom - scale(FOOTER_HEIGHT),
            rgb(38, 49, 54),
        );
        line(
            hdc,
            0,
            client.bottom - scale(FOOTER_HEIGHT),
            client.right,
            client.bottom - scale(FOOTER_HEIGHT),
            rgb(38, 49, 54),
        );

        let old = SelectObject(hdc, HGDIOBJ(state.title_font.0));
        SetBkMode(hdc, TRANSPARENT);
        SetTextColor(hdc, rgb(239, 246, 247));
        let mut title = wide(state.language.text("settings"));
        let title_len = title.len() - 1;
        let mut title_rect = RECT {
            left: scale(18),
            top: scale(14),
            right: scale(690),
            bottom: scale(42),
        };
        DrawTextW(
            hdc,
            &mut title[..title_len],
            &mut title_rect,
            DT_LEFT | DT_SINGLELINE | DT_VCENTER | DT_NOPREFIX,
        );
        SelectObject(hdc, HGDIOBJ(state.font.0));
        SetTextColor(hdc, rgb(126, 146, 151));
        let mut path = wide(&state.config_path.to_string_lossy());
        let path_len = path.len() - 1;
        let mut path_rect = RECT {
            left: scale(18),
            top: scale(40),
            right: scale(690),
            bottom: scale(65),
        };
        DrawTextW(
            hdc,
            &mut path[..path_len],
            &mut path_rect,
            DT_LEFT | DT_SINGLELINE | DT_VCENTER | DT_END_ELLIPSIS | DT_NOPREFIX,
        );

        let active = GROUPS
            .get(state.active_group)
            .map(|group| group.0)
            .unwrap_or("Display");
        let focused = GetFocus();
        for frame in state
            .input_frames
            .iter()
            .filter(|frame| frame.group == active)
        {
            rounded_box(
                hdc,
                scaled_rect(frame.rect, state.dpi),
                rgb(26, 35, 39),
                if focused == frame.control {
                    rgb(92, 192, 176)
                } else {
                    rgb(54, 68, 74)
                },
                scale(7),
            );
        }
        if active == "About" {
            paint_about(state, hdc);
        }
        SelectObject(hdc, old);
        let _ = EndPaint(state.hwnd, &paint);
    }
}

fn draw_owner_button(state: &SettingsState, item: &DRAWITEMSTRUCT) {
    let id = item.CtlID as usize;
    if let Some(key) = state.boolean_ids.get(&id).copied() {
        unsafe {
            draw_checkbox(
                state,
                item,
                state.boolean_values.get(key).copied().unwrap_or(false),
            );
        }
        return;
    }

    let selected_page = id >= ID_PAGE_BASE && id == ID_PAGE_BASE + state.active_group;
    let pressed = item.itemState.0 & ODS_SELECTED.0 != 0;
    let hot = item.itemState.0 & ODS_HOTLIGHT.0 != 0;
    let focused = item.itemState.0 & ODS_FOCUS.0 != 0;
    let disabled = item.itemState.0 & ODS_DISABLED.0 != 0;
    let scale = |value| platform::scale(value, state.dpi);

    unsafe {
        let base = if id >= ID_PAGE_BASE && id < ID_PAGE_BASE + GROUPS.len() {
            rgb(12, 18, 21)
        } else if id == ID_SAVE || id == ID_CANCEL {
            rgb(13, 19, 22)
        } else {
            rgb(16, 22, 25)
        };
        fill_color(item.hDC, item.rcItem, base);
        if id == ID_CLOSE {
            rounded_box(
                item.hDC,
                item.rcItem,
                if pressed {
                    rgb(40, 51, 56)
                } else {
                    rgb(27, 36, 40)
                },
                if hot || focused {
                    rgb(82, 111, 111)
                } else {
                    rgb(52, 66, 72)
                },
                scale(8),
            );
            let cx = (item.rcItem.left + item.rcItem.right) / 2;
            let cy = (item.rcItem.top + item.rcItem.bottom) / 2;
            stroke_polyline(
                item.hDC,
                &[
                    POINT {
                        x: cx - scale(5),
                        y: cy - scale(5),
                    },
                    POINT {
                        x: cx + scale(5),
                        y: cy + scale(5),
                    },
                ],
                rgb(159, 218, 209),
                scale(2).max(1),
            );
            stroke_polyline(
                item.hDC,
                &[
                    POINT {
                        x: cx + scale(5),
                        y: cy - scale(5),
                    },
                    POINT {
                        x: cx - scale(5),
                        y: cy + scale(5),
                    },
                ],
                rgb(159, 218, 209),
                scale(2).max(1),
            );
            return;
        }

        if id == ID_LANGUAGE {
            rounded_box(
                item.hDC,
                item.rcItem,
                if pressed {
                    rgb(31, 43, 47)
                } else {
                    rgb(26, 35, 39)
                },
                if state.language_open || focused || hot {
                    rgb(82, 171, 158)
                } else {
                    rgb(54, 68, 74)
                },
                scale(7),
            );
            draw_button_text(
                state,
                item,
                rgb(234, 242, 243),
                DT_LEFT,
                scale(14),
                scale(42),
            );
            let cx = item.rcItem.right - scale(17);
            let cy = (item.rcItem.top + item.rcItem.bottom) / 2;
            let direction = if state.language_open { -1 } else { 1 };
            stroke_polyline(
                item.hDC,
                &[
                    POINT {
                        x: cx - scale(4),
                        y: cy - direction * scale(2),
                    },
                    POINT {
                        x: cx,
                        y: cy + direction * scale(2),
                    },
                    POINT {
                        x: cx + scale(4),
                        y: cy - direction * scale(2),
                    },
                ],
                rgb(185, 207, 207),
                scale(1).max(1),
            );
            return;
        }

        if id >= ID_LANGUAGE_ITEM_BASE && id < ID_LANGUAGE_ITEM_BASE + Language::ALL.len() {
            let index = id - ID_LANGUAGE_ITEM_BASE;
            let selected = Language::ALL.get(index).copied() == Some(state.language);
            rounded_box(
                item.hDC,
                item.rcItem,
                if pressed || hot {
                    rgb(37, 57, 59)
                } else if selected {
                    rgb(29, 51, 52)
                } else {
                    rgb(23, 31, 35)
                },
                if selected {
                    rgb(70, 142, 132)
                } else {
                    rgb(47, 61, 66)
                },
                scale(6),
            );
            draw_button_text(
                state,
                item,
                if selected {
                    rgb(231, 250, 246)
                } else {
                    rgb(190, 205, 208)
                },
                DT_LEFT,
                scale(14),
                scale(42),
            );
            if selected {
                let cx = item.rcItem.right - scale(18);
                let cy = (item.rcItem.top + item.rcItem.bottom) / 2;
                stroke_polyline(
                    item.hDC,
                    &[
                        POINT {
                            x: cx - scale(5),
                            y: cy,
                        },
                        POINT {
                            x: cx - scale(1),
                            y: cy + scale(4),
                        },
                        POINT {
                            x: cx + scale(6),
                            y: cy - scale(5),
                        },
                    ],
                    rgb(112, 215, 195),
                    scale(2).max(1),
                );
            }
            return;
        }

        let (fill, border, text_color, radius) = if id == ID_SAVE {
            (
                if disabled {
                    rgb(54, 86, 81)
                } else if pressed {
                    rgb(80, 187, 169)
                } else {
                    rgb(112, 215, 195)
                },
                if disabled {
                    rgb(54, 86, 81)
                } else {
                    rgb(112, 215, 195)
                },
                if disabled {
                    rgb(118, 145, 140)
                } else {
                    rgb(7, 28, 24)
                },
                7,
            )
        } else if id == ID_CANCEL {
            (
                if pressed {
                    rgb(36, 47, 52)
                } else {
                    rgb(26, 35, 39)
                },
                if focused || hot {
                    rgb(82, 102, 109)
                } else {
                    rgb(56, 70, 76)
                },
                if disabled {
                    rgb(92, 105, 109)
                } else {
                    rgb(215, 226, 228)
                },
                7,
            )
        } else if selected_page {
            (rgb(27, 51, 52), rgb(55, 115, 106), rgb(237, 248, 246), 7)
        } else {
            (
                if pressed || hot {
                    rgb(22, 31, 35)
                } else {
                    rgb(12, 18, 21)
                },
                rgb(12, 18, 21),
                if disabled {
                    rgb(83, 96, 100)
                } else {
                    rgb(166, 183, 187)
                },
                7,
            )
        };
        rounded_box(item.hDC, item.rcItem, fill, border, scale(radius));
        if selected_page {
            fill_color(
                item.hDC,
                RECT {
                    left: item.rcItem.left,
                    top: item.rcItem.top + scale(7),
                    right: item.rcItem.left + scale(3),
                    bottom: item.rcItem.bottom - scale(7),
                },
                rgb(112, 215, 195),
            );
        }
        SetBkMode(item.hDC, TRANSPARENT);
        draw_button_text(
            state,
            item,
            text_color,
            if id >= ID_PAGE_BASE && id < ID_PAGE_BASE + GROUPS.len() {
                DT_LEFT
            } else {
                DT_CENTER
            },
            if id >= ID_PAGE_BASE && id < ID_PAGE_BASE + GROUPS.len() {
                scale(11)
            } else {
                0
            },
            0,
        );
    }
}

unsafe fn draw_checkbox(state: &SettingsState, item: &DRAWITEMSTRUCT, checked: bool) {
    let scale = |value| platform::scale(value, state.dpi);
    let side = scale(20);
    let left = item.rcItem.left + (item.rcItem.right - item.rcItem.left - side) / 2;
    let top = item.rcItem.top + (item.rcItem.bottom - item.rcItem.top - side) / 2;
    let rect = RECT {
        left,
        top,
        right: left + side,
        bottom: top + side,
    };
    let pressed = item.itemState.0 & ODS_SELECTED.0 != 0;
    let focused = item.itemState.0 & ODS_FOCUS.0 != 0;
    let disabled = item.itemState.0 & ODS_DISABLED.0 != 0;
    unsafe {
        fill_color(item.hDC, item.rcItem, rgb(16, 22, 25));
        rounded_box(
            item.hDC,
            rect,
            if disabled {
                rgb(29, 39, 43)
            } else if checked {
                if pressed {
                    rgb(82, 188, 170)
                } else {
                    rgb(112, 215, 195)
                }
            } else if pressed {
                rgb(35, 47, 52)
            } else {
                rgb(24, 33, 37)
            },
            if disabled {
                rgb(48, 61, 66)
            } else if checked || focused {
                rgb(112, 215, 195)
            } else {
                rgb(67, 82, 88)
            },
            scale(5),
        );
        if checked {
            let cx = (rect.left + rect.right) / 2;
            let cy = (rect.top + rect.bottom) / 2;
            stroke_polyline(
                item.hDC,
                &[
                    POINT {
                        x: cx - scale(5),
                        y: cy,
                    },
                    POINT {
                        x: cx - scale(1),
                        y: cy + scale(4),
                    },
                    POINT {
                        x: cx + scale(6),
                        y: cy - scale(5),
                    },
                ],
                if disabled {
                    rgb(88, 108, 105)
                } else {
                    rgb(7, 31, 26)
                },
                scale(2).max(1),
            );
        }
    }
}

unsafe fn draw_button_text(
    state: &SettingsState,
    item: &DRAWITEMSTRUCT,
    color: COLORREF,
    alignment: windows::Win32::Graphics::Gdi::DRAW_TEXT_FORMAT,
    left_padding: i32,
    right_padding: i32,
) {
    let text = read_text(item.hwndItem);
    let mut text = text.encode_utf16().collect::<Vec<_>>();
    let mut rect = item.rcItem;
    rect.left += left_padding;
    rect.right -= right_padding;
    unsafe {
        let old = SelectObject(item.hDC, HGDIOBJ(state.font.0));
        SetBkMode(item.hDC, TRANSPARENT);
        SetTextColor(item.hDC, color);
        DrawTextW(
            item.hDC,
            &mut text,
            &mut rect,
            alignment | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX | DT_END_ELLIPSIS,
        );
        SelectObject(item.hDC, old);
    }
}

unsafe fn paint_about(state: &SettingsState, hdc: HDC) {
    let s = |value| platform::scale(value, state.dpi);
    unsafe {
        if let Ok(icon) = resources::load_app_icon_sized(s(70), s(70)) {
            let _ = DrawIconEx(hdc, s(196), s(96), icon, s(70), s(70), 0, None, DI_NORMAL);
        }
        draw_text_line(
            hdc,
            state.title_font,
            "STT for Windows",
            rgb(239, 246, 247),
            RECT {
                left: s(286),
                top: s(96),
                right: s(732),
                bottom: s(126),
            },
            DT_LEFT,
        );
        draw_text_line(
            hdc,
            state.font,
            concat!("Version ", env!("CARGO_PKG_VERSION")),
            rgb(112, 215, 195),
            RECT {
                left: s(286),
                top: s(129),
                right: s(732),
                bottom: s(151),
            },
            DT_LEFT,
        );
        draw_text_line(
            hdc,
            state.small_font,
            "Native speech-to-text workflow for Windows",
            rgb(132, 151, 156),
            RECT {
                left: s(286),
                top: s(151),
                right: s(732),
                bottom: s(171),
            },
            DT_LEFT,
        );

        let card = RECT {
            left: s(196),
            top: s(194),
            right: s(732),
            bottom: s(400),
        };
        rounded_box(hdc, card, rgb(20, 28, 32), rgb(45, 58, 64), s(10));
        let rows = [
            ("Author", "Joey Kot", false),
            ("Email", "joey.kot.x@gmail.com", false),
            ("License", "GPL-3.0-or-later", false),
            ("Repository", "github.com/Joey-Kot/STT-for-Windows", true),
        ];
        for (index, (label, value, accent)) in rows.iter().enumerate() {
            let top = 206 + index as i32 * 46;
            draw_text_line(
                hdc,
                state.small_font,
                label,
                rgb(116, 137, 142),
                RECT {
                    left: s(216),
                    top: s(top),
                    right: s(310),
                    bottom: s(top + 30),
                },
                DT_LEFT,
            );
            draw_text_line(
                hdc,
                state.font,
                value,
                if *accent {
                    rgb(112, 215, 195)
                } else {
                    rgb(217, 228, 230)
                },
                RECT {
                    left: s(316),
                    top: s(top),
                    right: s(710),
                    bottom: s(top + 30),
                },
                DT_LEFT,
            );
            if index + 1 < rows.len() {
                line(
                    hdc,
                    s(216),
                    s(top + 38),
                    s(712),
                    s(top + 38),
                    rgb(39, 50, 55),
                );
            }
        }
    }
}

fn refresh_language(state: &SettingsState) {
    unsafe {
        let _ = SetWindowTextW(
            state.hwnd,
            PCWSTR(wide(state.language.text("settings")).as_ptr()),
        );
    }
    for (hwnd, key) in &state.localized_controls {
        let text = if *key == "API" {
            "API"
        } else {
            state.language.text(key)
        };
        unsafe {
            let _ = SetWindowTextW(*hwnd, PCWSTR(wide(text).as_ptr()));
            let _ = InvalidateRect(Some(*hwnd), None, true);
        }
    }
    unsafe {
        let _ = SetWindowTextW(
            state.language_control,
            PCWSTR(wide(state.language.native_name()).as_ptr()),
        );
        let _ = InvalidateRect(Some(state.language_control), None, true);
        let _ = InvalidateRect(Some(state.hwnd), None, true);
    }
}

fn update_page_visibility(state: &SettingsState) {
    let active = GROUPS
        .get(state.active_group)
        .map(|group| group.0)
        .unwrap_or("Display");
    unsafe {
        for (handle, group) in &state.control_groups {
            let hwnd = HWND(*handle as *mut c_void);
            let _ = ShowWindow(hwnd, if *group == active { SW_SHOW } else { SW_HIDE });
        }
    }
    if active != "Display" || !state.language_open {
        for item in &state.language_items {
            unsafe {
                let _ = ShowWindow(*item, SW_HIDE);
            }
        }
    }
}

fn set_language_dropdown(state: &mut SettingsState, open: bool) {
    state.language_open = open && state.active_group == 0;
    for item in &state.language_items {
        unsafe {
            let _ = ShowWindow(
                *item,
                if state.language_open {
                    SW_SHOW
                } else {
                    SW_HIDE
                },
            );
            if state.language_open {
                let _ = SetWindowPos(
                    *item,
                    Some(HWND_TOP),
                    0,
                    0,
                    0,
                    0,
                    SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE,
                );
                let _ = InvalidateRect(Some(*item), None, true);
            }
        }
    }
    unsafe {
        let _ = InvalidateRect(Some(state.language_control), None, true);
    }
}

fn select_language(state: &mut SettingsState, index: usize) {
    let Some(language) = Language::ALL.get(index).copied() else {
        return;
    };
    state.language = language;
    language.save();
    set_language_dropdown(state, false);
    refresh_language(state);
    for item in &state.language_items {
        unsafe {
            let _ = InvalidateRect(Some(*item), None, true);
        }
    }
    unsafe {
        let _ = PostMessageW(
            Some(state.owner),
            WM_LANGUAGE_CHANGED,
            WPARAM(index),
            LPARAM(0),
        );
    }
}

fn save(state: &mut SettingsState) {
    if state.saving {
        return;
    }
    if !state.runtime.can_reload() {
        show_error(
            state.hwnd,
            "Settings can only be saved while idle or in Error state.",
        );
        return;
    }
    let config = match read_config(state) {
        Ok(config) => config,
        Err(error) => {
            show_error(state.hwnd, &error);
            return;
        }
    };
    if let Err(error) = config.validate() {
        show_error(state.hwnd, &error.to_string());
        return;
    }
    state.saving = true;
    enable_controls(state, false);
    let runtime = state.runtime.clone();
    let config_path = state.config_path.clone();
    let hwnd = state.hwnd.0 as usize;
    std::thread::spawn(move || {
        let result = (|| -> Result<Event, String> {
            config
                .save(&config_path)
                .map_err(|error| error.to_string())?;
            let async_runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| error.to_string())?;
            async_runtime
                .block_on(runtime.reload(config))
                .map_err(|error| error.to_string())?;
            Ok(runtime.snapshot())
        })();
        let pointer = Box::into_raw(Box::new(result));
        let hwnd = HWND(hwnd as *mut c_void);
        if unsafe {
            PostMessageW(
                Some(hwnd),
                WM_SAVE_RESULT,
                WPARAM(0),
                LPARAM(pointer as isize),
            )
        }
        .is_err()
        {
            unsafe {
                drop(Box::from_raw(pointer));
            }
        }
    });
}

fn read_config(state: &SettingsState) -> Result<Config, String> {
    let mut value =
        serde_json::to_value(state.runtime.config()).map_err(|error| error.to_string())?;
    let object = value
        .as_object_mut()
        .ok_or("config serialization was not an object")?;
    for field in FIELDS {
        let hwnd = state.controls[field.key];
        let value = match field.kind {
            FieldKind::Boolean => serde_json::Value::Bool(
                state
                    .boolean_values
                    .get(field.key)
                    .copied()
                    .unwrap_or(false),
            ),
            FieldKind::Integer => serde_json::Value::Number(
                read_text(hwnd)
                    .trim()
                    .parse::<i64>()
                    .map_err(|_| format!("{} must be an integer", field.label))?
                    .into(),
            ),
            FieldKind::Float => serde_json::Number::from_f64(
                read_text(hwnd)
                    .trim()
                    .parse::<f64>()
                    .map_err(|_| format!("{} must be a number", field.label))?,
            )
            .map(serde_json::Value::Number)
            .ok_or_else(|| format!("{} is not a finite number", field.label))?,
            _ => serde_json::Value::String(read_text(hwnd)),
        };
        object.insert(field.key.into(), value);
    }
    let config: Config = serde_json::from_value(value).map_err(|error| error.to_string())?;
    if !config.extra_config.is_empty() {
        serde_json::from_str::<serde_json::Map<String, serde_json::Value>>(&config.extra_config)
            .map_err(|error| format!("Extra config is invalid JSON: {error}"))?;
    }
    Ok(config)
}

fn read_text(hwnd: HWND) -> String {
    let length = unsafe { GetWindowTextLengthW(hwnd) }.max(0) as usize;
    let mut buffer = vec![0_u16; length + 1];
    let copied = unsafe { GetWindowTextW(hwnd, &mut buffer) }.max(0) as usize;
    String::from_utf16_lossy(&buffer[..copied])
}

fn enable_controls(state: &SettingsState, enabled: bool) {
    for control in state
        .controls
        .values()
        .chain(state.page_buttons.iter())
        .chain(std::iter::once(&state.language_control))
        .chain(state.language_items.iter())
    {
        unsafe {
            let _ = EnableWindow(*control, enabled);
        }
    }
}

fn create_font(dpi: u32, points: i32, bold: bool) -> HFONT {
    unsafe {
        CreateFontW(
            -platform::scale(points, dpi),
            0,
            0,
            0,
            if bold {
                FW_BOLD.0 as i32
            } else {
                FW_NORMAL.0 as i32
            },
            0,
            0,
            0,
            DEFAULT_CHARSET,
            OUT_DEFAULT_PRECIS,
            CLIP_DEFAULT_PRECIS,
            CLEARTYPE_QUALITY,
            u32::from(DEFAULT_PITCH.0 | FF_DONTCARE.0),
            w!("Segoe UI"),
        )
    }
}

fn set_font(hwnd: HWND, font: HFONT) {
    unsafe {
        SendMessageW(
            hwnd,
            WM_SETFONT,
            Some(WPARAM(font.0 as usize)),
            Some(LPARAM(1)),
        );
    }
}

fn scaled_rect(rect: RECT, dpi: u32) -> RECT {
    RECT {
        left: platform::scale(rect.left, dpi),
        top: platform::scale(rect.top, dpi),
        right: platform::scale(rect.right, dpi),
        bottom: platform::scale(rect.bottom, dpi),
    }
}

unsafe fn rounded_box(hdc: HDC, rect: RECT, fill: COLORREF, border: COLORREF, radius: i32) {
    unsafe {
        let old_brush = SelectObject(hdc, GetStockObject(DC_BRUSH));
        let old_pen = SelectObject(hdc, GetStockObject(DC_PEN));
        SetDCBrushColor(hdc, fill);
        SetDCPenColor(hdc, border);
        let diameter = radius.max(1) * 2;
        let _ = RoundRect(
            hdc,
            rect.left,
            rect.top,
            rect.right,
            rect.bottom,
            diameter,
            diameter,
        );
        SelectObject(hdc, old_pen);
        SelectObject(hdc, old_brush);
    }
}

unsafe fn stroke_polyline(hdc: HDC, points: &[POINT], color: COLORREF, width: i32) {
    if points.len() < 2 {
        return;
    }
    unsafe {
        let pen = CreatePen(PS_SOLID, width.max(1), color);
        let old = SelectObject(hdc, HGDIOBJ(pen.0));
        let _ = MoveToEx(hdc, points[0].x, points[0].y, None);
        for point in &points[1..] {
            let _ = LineTo(hdc, point.x, point.y);
        }
        SelectObject(hdc, old);
        let _ = DeleteObject(HGDIOBJ(pen.0));
    }
}

unsafe fn draw_text_line(
    hdc: HDC,
    font: HFONT,
    text: &str,
    color: COLORREF,
    mut rect: RECT,
    alignment: windows::Win32::Graphics::Gdi::DRAW_TEXT_FORMAT,
) {
    let mut text = text.encode_utf16().collect::<Vec<_>>();
    unsafe {
        let old = SelectObject(hdc, HGDIOBJ(font.0));
        SetBkMode(hdc, TRANSPARENT);
        SetTextColor(hdc, color);
        DrawTextW(
            hdc,
            &mut text,
            &mut rect,
            alignment | DT_SINGLELINE | DT_VCENTER | DT_NOPREFIX | DT_END_ELLIPSIS,
        );
        SelectObject(hdc, old);
    }
}

unsafe fn fill_color(hdc: HDC, rect: RECT, color: COLORREF) {
    let brush = unsafe { CreateSolidBrush(color) };
    unsafe {
        FillRect(hdc, &rect, brush);
        let _ = DeleteObject(HGDIOBJ(brush.0));
    }
}

unsafe fn line(hdc: HDC, left: i32, top: i32, right: i32, bottom: i32, color: COLORREF) {
    if bottom == top {
        unsafe {
            fill_color(
                hdc,
                RECT {
                    left,
                    top,
                    right,
                    bottom: top + 1,
                },
                color,
            );
        }
    } else {
        unsafe {
            fill_color(
                hdc,
                RECT {
                    left,
                    top,
                    right: left + 1,
                    bottom,
                },
                color,
            );
        }
    }
}

fn rgb(red: u8, green: u8, blue: u8) -> COLORREF {
    COLORREF(u32::from(red) | (u32::from(green) << 8) | (u32::from(blue) << 16))
}

fn show_error(owner: HWND, message: &str) {
    let message = wide(message);
    unsafe {
        let _ = MessageBoxW(
            Some(owner),
            PCWSTR(message.as_ptr()),
            w!("STT Settings"),
            MB_OK | MB_ICONERROR,
        );
    }
}

fn wide(text: &str) -> Vec<u16> {
    text.encode_utf16().chain(std::iter::once(0)).collect()
}
