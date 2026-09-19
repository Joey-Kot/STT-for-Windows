//! Shared rounded menu surface for the language and microphone selectors.
use super::*;
use windows::Win32::Graphics::Gdi::{
    BitBlt, CreateCompatibleBitmap, CreateCompatibleDC, CreateRoundRectRgn, DeleteDC, SRCCOPY,
    SetViewportOrgEx,
};
use windows::Win32::UI::Controls::WM_MOUSELEAVE;
use windows::Win32::UI::Input::KeyboardAndMouse::{TME_LEAVE, TRACKMOUSEEVENT, TrackMouseEvent};
use windows::Win32::UI::Shell::{DefSubclassProc, GetWindowSubclass, SetWindowSubclass};
use windows::Win32::UI::WindowsAndMessaging::{
    GetCursorPos, LB_GETITEMRECT, LB_ITEMFROMPOINT, LB_RESETCONTENT, LB_SETTOPINDEX, WM_CHAR,
    WM_KEYDOWN, WM_MOUSEMOVE, WM_MOUSEWHEEL, WM_VSCROLL,
};

pub(super) const PADDING: i32 = 8;
pub(super) const ROW_HEIGHT: i32 = 36;

pub(super) fn background() -> COLORREF {
    rgb(23, 31, 35)
}

pub(super) fn track_hover(hwnd: HWND, list: bool) -> Result<(), String> {
    // Bit zero identifies list controls; remaining bits store hovered index + 1.
    if unsafe { SetWindowSubclass(hwnd, Some(hover_proc), 2, usize::from(list)) }.as_bool() {
        Ok(())
    } else {
        Err("Unable to initialize dropdown hover tracking".into())
    }
}

pub(super) fn hovered(hwnd: HWND) -> bool {
    let mut hot = 0;
    unsafe {
        GetWindowSubclass(hwnd, Some(hover_proc), 2, Some(&mut hot)).as_bool() && hot >> 1 != 0
    }
}

pub(super) fn hovered_row(hwnd: HWND, index: u32) -> bool {
    let mut hot = 0;
    unsafe {
        GetWindowSubclass(hwnd, Some(hover_proc), 2, Some(&mut hot)).as_bool()
            && hot >> 1 == index as usize + 1
    }
}

fn invalidate_hover(hwnd: HWND, list: bool, row: usize) {
    if row == 0 {
        return;
    }
    unsafe {
        if list {
            let mut rect = RECT::default();
            if SendMessageW(
                hwnd,
                LB_GETITEMRECT,
                Some(WPARAM(row - 1)),
                Some(LPARAM(&mut rect as *mut RECT as isize)),
            )
            .0 >= 0
            {
                let _ = InvalidateRect(Some(hwnd), Some(&rect), false);
            }
        } else {
            let _ = InvalidateRect(Some(hwnd), None, false);
        }
    }
}

unsafe extern "system" fn hover_proc(
    hwnd: HWND,
    message: u32,
    wparam: WPARAM,
    lparam: LPARAM,
    _: usize,
    data: usize,
) -> LRESULT {
    unsafe {
        let result = DefSubclassProc(hwnd, message, wparam, lparam);
        if matches!(
            message,
            WM_MOUSEMOVE
                | WM_MOUSELEAVE
                | WM_MOUSEWHEEL
                | WM_VSCROLL
                | LB_RESETCONTENT
                | LB_SETTOPINDEX
                | WM_KEYDOWN
                | WM_CHAR
        ) && (data & 1 != 0 || !matches!(message, WM_KEYDOWN | WM_CHAR))
        {
            let list = data & 1 != 0;
            let old = data >> 1;
            let mut row = 0;
            if !matches!(message, WM_MOUSELEAVE | LB_RESETCONTENT) {
                if list {
                    let mut cursor = POINT::default();
                    if GetCursorPos(&mut cursor).is_ok()
                        && ScreenToClient(hwnd, &mut cursor).as_bool()
                    {
                        let point = LPARAM(
                            ((cursor.y as u16 as u32) << 16 | cursor.x as u16 as u32) as isize,
                        );
                        let hit =
                            SendMessageW(hwnd, LB_ITEMFROMPOINT, None, Some(point)).0 as usize;
                        if hit >> 16 == 0 {
                            row = (hit & 0xffff) + 1;
                        }
                    }
                } else {
                    row = 1;
                }
            }
            if message == WM_MOUSEMOVE && old == 0 {
                let _ = TrackMouseEvent(&mut TRACKMOUSEEVENT {
                    cbSize: size_of::<TRACKMOUSEEVENT>() as u32,
                    dwFlags: TME_LEAVE,
                    hwndTrack: hwnd,
                    dwHoverTime: 0,
                });
            }
            if old != row {
                let _ =
                    SetWindowSubclass(hwnd, Some(hover_proc), 2, (row << 1) | usize::from(list));
                invalidate_hover(hwnd, list, old);
                invalidate_hover(hwnd, list, row);
            }
        }
        result
    }
}

/// Compose background, highlight and text offscreen before copying the row.
unsafe fn draw_buffered(parent: HWND, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
    unsafe {
        let item = &*(lparam.0 as *const DRAWITEMSTRUCT);
        let rect = item.rcItem;
        let width = rect.right - rect.left;
        let height = rect.bottom - rect.top;
        if width <= 0 || height <= 0 {
            return LRESULT(1);
        }
        let dc = CreateCompatibleDC(Some(item.hDC));
        let bitmap = CreateCompatibleBitmap(item.hDC, width, height);
        if dc.0.is_null() || bitmap.0.is_null() {
            let _ = DeleteDC(dc);
            let _ = DeleteObject(HGDIOBJ(bitmap.0));
            return SendMessageW(parent, WM_DRAWITEM, Some(wparam), Some(lparam));
        }
        let old = SelectObject(dc, HGDIOBJ(bitmap.0));
        let _ = SetViewportOrgEx(dc, -rect.left, -rect.top, None);
        // Empty owner-draw list notifications may have no row content.
        SetDCBrushColor(dc, background());
        FillRect(dc, &rect, HBRUSH(GetStockObject(DC_BRUSH).0));
        let mut buffered = *item;
        buffered.hDC = dc;
        let result = SendMessageW(
            parent,
            WM_DRAWITEM,
            Some(wparam),
            Some(LPARAM(&buffered as *const DRAWITEMSTRUCT as isize)),
        );
        let _ = BitBlt(
            item.hDC,
            rect.left,
            rect.top,
            width,
            height,
            Some(dc),
            rect.left,
            rect.top,
            SRCCOPY,
        );
        SelectObject(dc, old);
        let _ = DeleteObject(HGDIOBJ(bitmap.0));
        let _ = DeleteDC(dc);
        result
    }
}

pub(super) fn create_panel(
    state: &SettingsState,
    instance: windows::Win32::Foundation::HMODULE,
) -> Result<HWND, String> {
    let panel = create_child(
        state,
        w!("STATIC"),
        "",
        WS_CHILD | WS_CLIPCHILDREN,
        0,
        0,
        EDIT_WIDTH,
        1,
        0,
        instance,
    )?;
    if !unsafe { SetWindowSubclass(panel, Some(panel_proc), 1, 0) }.as_bool() {
        return Err("Unable to initialize dropdown panel".into());
    }
    Ok(panel)
}

/// Returns the inner width, anchored to the button's actual bounds at any DPI.
pub(super) fn position(panel: HWND, button: HWND, height: i32, dpi: u32) -> i32 {
    position_pixels(panel, button, platform::scale(height, dpi), dpi)
}

pub(super) fn position_pixels(panel: HWND, button: HWND, height: i32, dpi: u32) -> i32 {
    unsafe {
        let mut bounds = RECT::default();
        let Ok(parent) = windows::Win32::UI::WindowsAndMessaging::GetParent(panel) else {
            return 0;
        };
        if GetWindowRect(button, &mut bounds).is_err() {
            return 0;
        }
        let mut origin = POINT {
            x: bounds.left,
            y: bounds.bottom,
        };
        if !ScreenToClient(parent, &mut origin).as_bool() {
            return 0;
        }
        let width = bounds.right - bounds.left;
        let height = height + platform::scale(PADDING * 2, dpi);
        let gap = platform::scale(6, dpi);
        let mut client = RECT::default();
        let _ = GetClientRect(parent, &mut client);
        // Audio fields near the footer need to open upward. Keep the entire menu
        // inside the settings content area instead of clipping its final rows.
        let below = origin.y + gap;
        let top = if below + height > client.bottom - platform::scale(FOOTER_HEIGHT, dpi) {
            (origin.y - (bounds.bottom - bounds.top) - gap - height)
                .max(platform::scale(HEADER_HEIGHT, dpi))
        } else {
            below
        };
        let _ = SetWindowPos(
            panel,
            Some(HWND_TOP),
            origin.x,
            top,
            width,
            height,
            SWP_NOACTIVATE,
        );
        let radius = platform::scale(12, dpi);
        let region = CreateRoundRectRgn(0, 0, width + 1, height + 1, radius * 2, radius * 2);
        if SetWindowRgn(panel, Some(region), true) == 0 {
            let _ = DeleteObject(HGDIOBJ(region.0));
        }
        let _ = InvalidateRect(Some(panel), None, true);
        (width - platform::scale(PADDING * 2, dpi)).max(1)
    }
}

pub(super) fn draw_row(hdc: HDC, rect: RECT, selected: bool, hot: bool, dpi: u32) {
    unsafe {
        SetDCBrushColor(hdc, background());
        FillRect(hdc, &rect, HBRUSH(GetStockObject(DC_BRUSH).0));
        if selected || hot {
            let color = if selected {
                rgb(40, 78, 73)
            } else {
                rgb(33, 49, 51)
            };
            let mut inset = rect;
            inset.top += platform::scale(2, dpi);
            inset.bottom -= platform::scale(2, dpi);
            rounded_box(hdc, inset, color, color, platform::scale(8, dpi));
        }
    }
}

unsafe extern "system" fn panel_proc(
    hwnd: HWND,
    message: u32,
    wparam: WPARAM,
    lparam: LPARAM,
    _: usize,
    _: usize,
) -> LRESULT {
    use windows::Win32::UI::WindowsAndMessaging::{GetParent, WM_MEASUREITEM};
    unsafe {
        match message {
            WM_NCHITTEST => return LRESULT(HTCLIENT as isize),
            WM_DRAWITEM => {
                if let Ok(parent) = GetParent(hwnd) {
                    return draw_buffered(parent, wparam, lparam);
                }
            }
            WM_COMMAND | WM_MEASUREITEM => {
                if let Ok(parent) = GetParent(hwnd) {
                    return SendMessageW(parent, message, Some(wparam), Some(lparam));
                }
            }
            WM_CTLCOLORLISTBOX | WM_CTLCOLORBTN => {
                let dc = HDC(wparam.0 as *mut c_void);
                SetBkColor(dc, background());
                SetDCBrushColor(dc, background());
                return LRESULT(GetStockObject(DC_BRUSH).0 as isize);
            }
            WM_ERASEBKGND => return LRESULT(1),
            WM_PAINT => {
                let mut paint = PAINTSTRUCT::default();
                let dc = BeginPaint(hwnd, &mut paint);
                SetDCBrushColor(dc, background());
                FillRect(dc, &paint.rcPaint, HBRUSH(GetStockObject(DC_BRUSH).0));
                let _ = EndPaint(hwnd, &paint);
                return LRESULT(0);
            }
            _ => {}
        }
        DefSubclassProc(hwnd, message, wparam, lparam)
    }
}
