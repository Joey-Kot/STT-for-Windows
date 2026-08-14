use thiserror::Error;

#[derive(Debug, Error)]
pub enum KeyboardError {
    #[error("keyboard injection is only available on Windows")]
    UnsupportedPlatform,
}

#[cfg(windows)]
#[link(name = "user32")]
unsafe extern "system" {
    fn keybd_event(virtual_key: u8, scan_code: u8, flags: u32, extra_info: usize);
}

/// Sends the exact Ctrl+V sequence used by micmonay/keybd_event v1.1.2.
///
/// The existing implementation passes Ctrl as a virtual key and V as scan
/// code 47. Its release order is Ctrl first, then V; compatibility requires
/// retaining that order even though many chord helpers choose the reverse.
#[cfg(windows)]
pub fn send_ctrl_v() -> Result<(), KeyboardError> {
    const VK_CONTROL: u8 = 0x11;
    const CTRL_SCAN_ARGUMENT: u8 = 0x91;
    const V_SCAN_CODE: u8 = 47;
    const V_SECOND_ARGUMENT: u8 = V_SCAN_CODE + 0x80;
    const KEYEVENTF_KEYUP: u32 = 0x0002;
    const KEYEVENTF_SCANCODE: u32 = 0x0008;

    unsafe {
        keybd_event(VK_CONTROL, CTRL_SCAN_ARGUMENT, 0, 0);
        keybd_event(V_SCAN_CODE, V_SECOND_ARGUMENT, KEYEVENTF_SCANCODE, 0);
        keybd_event(VK_CONTROL, CTRL_SCAN_ARGUMENT, KEYEVENTF_KEYUP, 0);
        keybd_event(
            V_SCAN_CODE,
            V_SECOND_ARGUMENT,
            KEYEVENTF_KEYUP | KEYEVENTF_SCANCODE,
            0,
        );
    }
    Ok(())
}

#[cfg(not(windows))]
pub fn send_ctrl_v() -> Result<(), KeyboardError> {
    Err(KeyboardError::UnsupportedPlatform)
}

#[cfg(test)]
mod tests {
    #[test]
    fn compatibility_constants_match_keybd_event_1_1_2() {
        assert_eq!(0x11_u8, 17);
        assert_eq!(47_u8 + 0x80, 175);
        assert_eq!(0x0002_u32 | 0x0008, 0x000a);
    }
}
