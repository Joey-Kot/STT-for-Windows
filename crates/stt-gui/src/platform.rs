use std::ffi::c_void;
#[cfg(feature = "static-libav")]
use std::ffi::{CStr, CString, c_char};
use std::path::PathBuf;

use async_trait::async_trait;
use stt_core::Config;
use stt_core::converter::{AudioConverter, ConvertError, paths_equal, settings_for};
use tokio_util::sync::CancellationToken;
use windows::Win32::Foundation::HWND;
use windows::Win32::Graphics::Dwm::{
    DWMWA_BORDER_COLOR, DWMWA_COLOR_NONE, DWMWA_USE_IMMERSIVE_DARK_MODE,
    DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_DONOTROUND, DwmSetWindowAttribute,
};
use windows::Win32::UI::HiDpi::{
    DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, GetDpiForSystem, GetDpiForWindow,
    SetProcessDpiAwarenessContext,
};

pub fn enable_high_dpi() {
    unsafe {
        let _ = SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    }
}

pub fn system_dpi() -> u32 {
    unsafe { GetDpiForSystem() }.max(96)
}

pub fn window_dpi(hwnd: HWND) -> u32 {
    unsafe { GetDpiForWindow(hwnd) }.max(96)
}

pub fn scale(value: i32, dpi: u32) -> i32 {
    ((i64::from(value) * i64::from(dpi) + 48) / 96) as i32
}

pub fn scale_with_factor(value: i32, dpi: u32, factor: f64) -> i32 {
    (f64::from(value) * f64::from(dpi) * factor / 96.0).round() as i32
}

pub fn unscale(value: i32, dpi: u32) -> i32 {
    ((i64::from(value) * 96 + i64::from(dpi / 2)) / i64::from(dpi.max(1))) as i32
}

pub fn unscale_with_factor(value: i32, dpi: u32, factor: f64) -> i32 {
    (f64::from(value) * 96.0 / (f64::from(dpi.max(1)) * factor)).round() as i32
}

pub fn window_opacity_alpha(opacity: f64) -> u8 {
    (opacity.clamp(0.1, 1.0) * 255.0).round() as u8
}

pub fn disable_native_window_frame(hwnd: HWND) {
    unsafe {
        let preference = DWMWCP_DONOTROUND;
        let _ = DwmSetWindowAttribute(
            hwnd,
            DWMWA_WINDOW_CORNER_PREFERENCE,
            &preference as *const _ as *const c_void,
            std::mem::size_of_val(&preference) as u32,
        );
        let border_color = DWMWA_COLOR_NONE;
        let _ = DwmSetWindowAttribute(
            hwnd,
            DWMWA_BORDER_COLOR,
            &border_color as *const _ as *const c_void,
            std::mem::size_of_val(&border_color) as u32,
        );
    }
}

pub fn apply_dark_mode(hwnd: HWND) {
    let enabled = 1_i32;
    unsafe {
        let _ = DwmSetWindowAttribute(
            hwnd,
            DWMWA_USE_IMMERSIVE_DARK_MODE,
            &enabled as *const _ as *const c_void,
            std::mem::size_of_val(&enabled) as u32,
        );
    }
}

pub fn config_path() -> Result<PathBuf, String> {
    let app_data = std::env::var_os("APPDATA").ok_or("APPDATA is not available")?;
    Ok(PathBuf::from(app_data).join("stt").join("config.json"))
}

#[derive(Debug, Default)]
pub struct GuiLibAvConverter;

#[async_trait]
impl AudioConverter for GuiLibAvConverter {
    async fn convert(
        &self,
        cancellation: &CancellationToken,
        config: &Config,
        input: &std::path::Path,
        output: &std::path::Path,
        source_rate: i32,
    ) -> Result<(), ConvertError> {
        if cancellation.is_cancelled() {
            return Err(ConvertError::Canceled);
        }
        if paths_equal(input, output) {
            return Err(ConvertError::SamePath);
        }
        let settings = settings_for(config, source_rate)?;

        #[cfg(not(feature = "static-libav"))]
        {
            let _ = (settings, input, output);
            return Err(ConvertError::LibAvUnavailable);
        }

        #[cfg(feature = "static-libav")]
        {
            let input = CString::new(input.to_string_lossy().as_bytes()).map_err(|error| {
                ConvertError::Failed {
                    message: error.to_string(),
                }
            })?;
            let output = CString::new(output.to_string_lossy().as_bytes()).map_err(|error| {
                ConvertError::Failed {
                    message: error.to_string(),
                }
            })?;
            let codec = CString::new(settings.ffmpeg_codec.as_bytes()).map_err(|error| {
                ConvertError::Failed {
                    message: error.to_string(),
                }
            })?;
            let sample_format =
                CString::new(settings.sample_format.as_bytes()).map_err(|error| {
                    ConvertError::Failed {
                        message: error.to_string(),
                    }
                })?;
            let mut error_buffer = [0_i8; 4096];
            if config.ffmpeg_debug {
                eprintln!(
                    "[ffmpeg] libav convert: {} -> {} codec={} channels={} rate={} bitrate={}k sample_fmt={}",
                    input.to_string_lossy(),
                    output.to_string_lossy(),
                    settings.ffmpeg_codec,
                    settings.channels,
                    settings.sample_rate,
                    settings.bitrate,
                    settings.sample_format
                );
            }
            let result = unsafe {
                stt_ffmpeg_convert(
                    input.as_ptr(),
                    output.as_ptr(),
                    codec.as_ptr(),
                    settings.channels,
                    settings.sample_rate,
                    settings.bitrate,
                    i32::from(settings.codec_has_bitrate),
                    sample_format.as_ptr(),
                    i32::from(config.ffmpeg_debug),
                    error_buffer.as_mut_ptr(),
                    error_buffer.len() as i32,
                )
            };
            if cancellation.is_cancelled() {
                return Err(ConvertError::Canceled);
            }
            if result < 0 {
                let message = unsafe { CStr::from_ptr(error_buffer.as_ptr()) }
                    .to_string_lossy()
                    .into_owned();
                return Err(ConvertError::Failed {
                    message: if message.is_empty() {
                        format!("libav conversion failed: {result}")
                    } else {
                        message
                    },
                });
            }
            Ok(())
        }
    }
}

#[cfg(feature = "static-libav")]
unsafe extern "C" {
    fn stt_ffmpeg_convert(
        input: *const c_char,
        output: *const c_char,
        codec: *const c_char,
        channels: i32,
        sample_rate: i32,
        bitrate_kbps: i32,
        codec_has_bitrate: i32,
        sample_format: *const c_char,
        debug: i32,
        error_buffer: *mut c_char,
        error_buffer_size: i32,
    ) -> i32;
}
