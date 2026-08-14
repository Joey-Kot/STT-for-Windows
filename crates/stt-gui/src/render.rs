use windows::Win32::Foundation::{HWND, RECT};
use windows::Win32::Graphics::Direct2D::Common::{
    D2D_RECT_F, D2D_SIZE_F, D2D_SIZE_U, D2D1_ALPHA_MODE_IGNORE, D2D1_COLOR_F,
    D2D1_FIGURE_BEGIN_HOLLOW, D2D1_FIGURE_END_CLOSED, D2D1_PIXEL_FORMAT,
};
use windows::Win32::Graphics::Direct2D::{
    D2D1_ANTIALIAS_MODE_PER_PRIMITIVE, D2D1_ARC_SEGMENT, D2D1_ARC_SIZE_LARGE, D2D1_ARC_SIZE_SMALL,
    D2D1_CAP_STYLE_ROUND, D2D1_DASH_STYLE_SOLID, D2D1_DRAW_TEXT_OPTIONS_NONE,
    D2D1_FACTORY_TYPE_SINGLE_THREADED, D2D1_FEATURE_LEVEL_DEFAULT,
    D2D1_HWND_RENDER_TARGET_PROPERTIES, D2D1_LINE_JOIN_ROUND, D2D1_PRESENT_OPTIONS_NONE,
    D2D1_RENDER_TARGET_PROPERTIES, D2D1_RENDER_TARGET_TYPE_DEFAULT, D2D1_RENDER_TARGET_USAGE_NONE,
    D2D1_ROUNDED_RECT, D2D1_STROKE_STYLE_PROPERTIES, D2D1_SWEEP_DIRECTION_CLOCKWISE,
    D2D1_SWEEP_DIRECTION_COUNTER_CLOCKWISE, D2D1CreateFactory, ID2D1Factory, ID2D1GeometrySink,
    ID2D1HwndRenderTarget, ID2D1PathGeometry, ID2D1SolidColorBrush, ID2D1StrokeStyle,
};
use windows::Win32::Graphics::DirectWrite::{
    DWRITE_FACTORY_TYPE_SHARED, DWRITE_FONT_STRETCH_NORMAL, DWRITE_FONT_STYLE_NORMAL,
    DWRITE_FONT_WEIGHT_NORMAL, DWRITE_MEASURING_MODE_NATURAL, DWRITE_PARAGRAPH_ALIGNMENT_CENTER,
    DWRITE_TEXT_ALIGNMENT_CENTER, DWriteCreateFactory, IDWriteFactory, IDWriteTextFormat,
};
use windows::Win32::UI::WindowsAndMessaging::GetClientRect;
use windows::core::w;
use windows_numerics::{Matrix3x2, Vector2};

use stt_core::runtime::{Event, State};

use crate::i18n::Language;

pub const BUTTON_SIZE: i32 = 32;
pub const BUTTON_GAP: i32 = 6;
pub const FULL_WIDTH: i32 = 222;
pub const FULL_HEIGHT: i32 = 94;
pub const MINIMAL_WIDTH: i32 = 170;
pub const MINIMAL_HEIGHT: i32 = 46;

pub struct Renderer {
    factory: ID2D1Factory,
    target: Option<ID2D1HwndRenderTarget>,
    text_format: IDWriteTextFormat,
    stroke_style: ID2D1StrokeStyle,
    gear_geometry: ID2D1PathGeometry,
    dpi: u32,
}

impl Renderer {
    pub fn new() -> windows::core::Result<Self> {
        unsafe {
            let factory: ID2D1Factory = D2D1CreateFactory(D2D1_FACTORY_TYPE_SINGLE_THREADED, None)?;
            let write_factory: IDWriteFactory = DWriteCreateFactory(DWRITE_FACTORY_TYPE_SHARED)?;
            let text_format = write_factory.CreateTextFormat(
                w!("Segoe UI"),
                None,
                DWRITE_FONT_WEIGHT_NORMAL,
                DWRITE_FONT_STYLE_NORMAL,
                DWRITE_FONT_STRETCH_NORMAL,
                12.0,
                w!("en-us"),
            )?;
            text_format.SetTextAlignment(DWRITE_TEXT_ALIGNMENT_CENTER)?;
            text_format.SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER)?;
            let stroke_style = factory.CreateStrokeStyle(
                &D2D1_STROKE_STYLE_PROPERTIES {
                    startCap: D2D1_CAP_STYLE_ROUND,
                    endCap: D2D1_CAP_STYLE_ROUND,
                    dashCap: D2D1_CAP_STYLE_ROUND,
                    lineJoin: D2D1_LINE_JOIN_ROUND,
                    miterLimit: 10.0,
                    dashStyle: D2D1_DASH_STYLE_SOLID,
                    dashOffset: 0.0,
                },
                None,
            )?;
            let gear_geometry = create_gear_geometry(&factory)?;
            Ok(Self {
                factory,
                target: None,
                text_format,
                stroke_style,
                gear_geometry,
                dpi: 96,
            })
        }
    }

    pub fn discard_device_resources(&mut self) {
        self.target = None;
    }

    pub fn set_dpi(&mut self, dpi: u32) {
        self.dpi = dpi.max(96);
        if let Some(target) = &self.target {
            unsafe { target.SetDpi(self.dpi as f32, self.dpi as f32) };
        }
    }

    pub fn resize(&mut self, width: u32, height: u32) {
        if let Some(target) = &self.target {
            unsafe {
                let _ = target.Resize(&D2D_SIZE_U { width, height });
            }
        }
    }

    #[allow(clippy::too_many_arguments)]
    pub fn paint(
        &mut self,
        hwnd: HWND,
        event: &Event,
        minimal: bool,
        rounded: bool,
        language: Language,
        _pressed_button: Option<i32>,
        hover_lifts: &[f32; 5],
        animation_time: f32,
    ) -> windows::core::Result<()> {
        self.ensure_target(hwnd)?;
        let target = self.target.as_ref().unwrap();
        unsafe {
            target.BeginDraw();
            target.Clear(Some(&color(0.0, 0.0, 0.0, 1.0)));

            let panel_color = rgb8(18, 22, 25);
            let button_color = rgb8(39, 45, 48);
            let hover_color = rgb8(49, 57, 61);
            let disabled_color = rgb8(27, 32, 35);
            let active_color = rgb8(197, 50, 50);
            let panel = target.CreateSolidColorBrush(&panel_color, None)?;
            let panel_border = target.CreateSolidColorBrush(&rgb8(42, 45, 48), None)?;
            let border = target.CreateSolidColorBrush(&rgb8(65, 70, 73), None)?;
            let disabled_border = target.CreateSolidColorBrush(&rgb8(37, 42, 45), None)?;
            let active_border = target.CreateSolidColorBrush(&rgb8(239, 108, 108), None)?;
            let normal = target.CreateSolidColorBrush(&rgb8(217, 227, 229), None)?;
            let muted = target.CreateSolidColorBrush(&rgb8(102, 108, 110), None)?;
            let warning = target.CreateSolidColorBrush(&rgb8(240, 208, 115), None)?;
            let danger = target.CreateSolidColorBrush(&rgb8(255, 156, 141), None)?;
            let accent = target.CreateSolidColorBrush(&rgb8(168, 215, 208), None)?;
            let white = target.CreateSolidColorBrush(&rgb8(255, 255, 255), None)?;
            let handle = target.CreateSolidColorBrush(&rgb8(65, 69, 71), None)?;
            let status_brush = target.CreateSolidColorBrush(&rgb8(174, 189, 192), None)?;
            let wave_factor = if event.state == State::Paused {
                0.72
            } else {
                1.0
            };
            let wave_primary =
                target.CreateSolidColorBrush(&color(1.0, 1.0, 1.0, 0.96 * wave_factor), None)?;
            let wave_secondary =
                target.CreateSolidColorBrush(&color(1.0, 1.0, 1.0, 0.60 * wave_factor), None)?;

            let size = if minimal {
                (MINIMAL_WIDTH, MINIMAL_HEIGHT)
            } else {
                (FULL_WIDTH, FULL_HEIGHT)
            };
            fill_rounded_f(
                target,
                &panel,
                &panel_border,
                D2D_RECT_F {
                    left: 0.0,
                    top: 0.0,
                    right: size.0 as f32,
                    bottom: size.1 as f32,
                },
                if rounded { 8.0 } else { 0.0 },
            );

            if !minimal {
                fill_rounded_f(
                    target,
                    &handle,
                    &handle,
                    D2D_RECT_F {
                        left: 90.0,
                        top: 9.0,
                        right: 132.0,
                        bottom: 13.0,
                    },
                    if rounded { 3.0 } else { 0.0 },
                );
            }

            let count = if minimal { 4 } else { 5 };
            for index in 0..count {
                let active_record =
                    index == 0 && matches!(event.state, State::Recording | State::Paused);
                let is_disabled = button_is_disabled(index, event.state);
                let hover_progress = (-hover_lifts[index as usize]).clamp(0.0, 1.0);
                let base_color = if active_record {
                    active_color
                } else if is_disabled {
                    disabled_color
                } else {
                    button_color
                };
                let fill_color = if is_disabled {
                    base_color
                } else {
                    mix(base_color, hover_color, hover_progress)
                };
                let fill = target.CreateSolidColorBrush(&fill_color, None)?;
                let outline = if active_record && !is_disabled {
                    &active_border
                } else if is_disabled {
                    &disabled_border
                } else {
                    &border
                };
                let rect = button_rect_f(index, minimal, hover_lifts[index as usize]);
                fill_rounded_f(
                    target,
                    &fill,
                    outline,
                    rect,
                    if rounded { 8.0 } else { 0.0 },
                );
                let brush = if is_disabled {
                    &muted
                } else if active_record {
                    &white
                } else if index == 1 {
                    &warning
                } else if index == 2 {
                    &danger
                } else if index >= 3 {
                    &accent
                } else {
                    &normal
                };
                draw_icon(
                    target,
                    &self.stroke_style,
                    &self.gear_geometry,
                    index,
                    rect,
                    brush,
                    &wave_primary,
                    &wave_secondary,
                    event.state,
                    minimal,
                    animation_time,
                );
            }

            if !minimal {
                let status = if !event.error.is_empty() {
                    event.error.as_str()
                } else if event.message.is_empty() {
                    language.text("idle")
                } else {
                    language.text(&event.message)
                };
                let utf16: Vec<u16> = status.encode_utf16().collect();
                target.DrawText(
                    &utf16,
                    &self.text_format,
                    &D2D_RECT_F {
                        left: 10.0,
                        top: 57.0,
                        right: 212.0,
                        bottom: 75.0,
                    },
                    &status_brush,
                    D2D1_DRAW_TEXT_OPTIONS_NONE,
                    DWRITE_MEASURING_MODE_NATURAL,
                );
            }

            if let Err(error) = target.EndDraw(None, None) {
                self.discard_device_resources();
                return Err(error);
            }
        }
        Ok(())
    }

    fn ensure_target(&mut self, hwnd: HWND) -> windows::core::Result<()> {
        if self.target.is_some() {
            return Ok(());
        }
        let mut client = RECT::default();
        unsafe {
            GetClientRect(hwnd, &mut client)?;
            let properties = D2D1_RENDER_TARGET_PROPERTIES {
                r#type: D2D1_RENDER_TARGET_TYPE_DEFAULT,
                pixelFormat: D2D1_PIXEL_FORMAT {
                    format: Default::default(),
                    alphaMode: D2D1_ALPHA_MODE_IGNORE,
                },
                dpiX: self.dpi as f32,
                dpiY: self.dpi as f32,
                usage: D2D1_RENDER_TARGET_USAGE_NONE,
                minLevel: D2D1_FEATURE_LEVEL_DEFAULT,
            };
            self.target = Some(self.factory.CreateHwndRenderTarget(
                &properties,
                &D2D1_HWND_RENDER_TARGET_PROPERTIES {
                    hwnd,
                    pixelSize: D2D_SIZE_U {
                        width: (client.right - client.left).max(1) as u32,
                        height: (client.bottom - client.top).max(1) as u32,
                    },
                    presentOptions: D2D1_PRESENT_OPTIONS_NONE,
                },
            )?);
        }
        Ok(())
    }
}

pub fn button_rect(index: i32, minimal: bool) -> RECT {
    let count = if minimal { 4 } else { 5 };
    let total = count * BUTTON_SIZE + (count - 1) * BUTTON_GAP;
    let width = if minimal { MINIMAL_WIDTH } else { FULL_WIDTH };
    let left = (width - total) / 2 + index * (BUTTON_SIZE + BUTTON_GAP);
    let top = if minimal { 7 } else { 20 };
    RECT {
        left,
        top,
        right: left + BUTTON_SIZE,
        bottom: top + BUTTON_SIZE,
    }
}

pub fn hit_test_button(x: i32, y: i32, minimal: bool) -> Option<i32> {
    let count = if minimal { 4 } else { 5 };
    (0..count).find(|index| {
        let rect = button_rect(*index, minimal);
        x >= rect.left && x < rect.right && y >= rect.top && y < rect.bottom
    })
}

pub fn button_is_disabled(index: i32, state: State) -> bool {
    (index == 1 && !matches!(state, State::Recording | State::Paused))
        || (index == 2 && !matches!(state, State::Recording | State::Paused | State::Uploading))
        || (index == 0 && state == State::Uploading)
}

fn button_rect_f(index: i32, minimal: bool, lift: f32) -> D2D_RECT_F {
    let rect = button_rect(index, minimal);
    D2D_RECT_F {
        left: rect.left as f32,
        top: rect.top as f32 + lift,
        right: rect.right as f32,
        bottom: rect.bottom as f32 + lift,
    }
}

unsafe fn fill_rounded_f(
    target: &ID2D1HwndRenderTarget,
    fill: &ID2D1SolidColorBrush,
    border: &ID2D1SolidColorBrush,
    rect: D2D_RECT_F,
    radius: f32,
) {
    let rounded = D2D1_ROUNDED_RECT {
        rect,
        radiusX: radius,
        radiusY: radius,
    };
    unsafe {
        target.FillRoundedRectangle(&rounded, fill);
        target.DrawRoundedRectangle(&rounded, border, 1.0, None);
    }
}

#[allow(clippy::too_many_arguments)]
unsafe fn draw_icon(
    target: &ID2D1HwndRenderTarget,
    stroke_style: &ID2D1StrokeStyle,
    gear_geometry: &ID2D1PathGeometry,
    index: i32,
    rect: D2D_RECT_F,
    brush: &ID2D1SolidColorBrush,
    wave_primary: &ID2D1SolidColorBrush,
    wave_secondary: &ID2D1SolidColorBrush,
    state: State,
    minimal: bool,
    animation_time: f32,
) {
    let cx = (rect.left + rect.right) / 2.0;
    let cy = (rect.top + rect.bottom) / 2.0;
    unsafe {
        match index {
            0 => draw_microphone(
                target,
                stroke_style,
                cx,
                cy,
                brush,
                wave_primary,
                wave_secondary,
                state,
                animation_time,
            ),
            1 if state == State::Paused => {
                draw_polyline(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    &[
                        v(cx - 3.0, cy - 5.25),
                        v(cx - 3.0, cy + 5.25),
                        v(cx + 5.25, cy),
                        v(cx - 3.0, cy - 5.25),
                    ],
                );
            }
            1 => {
                draw_line(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    cx - 3.0,
                    cy - 5.25,
                    cx - 3.0,
                    cy + 5.25,
                );
                draw_line(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    cx + 3.0,
                    cy - 5.25,
                    cx + 3.0,
                    cy + 5.25,
                );
            }
            2 => {
                draw_line(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    cx - 4.5,
                    cy - 4.5,
                    cx + 4.5,
                    cy + 4.5,
                );
                draw_line(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    cx + 4.5,
                    cy - 4.5,
                    cx - 4.5,
                    cy + 4.5,
                );
            }
            3 if minimal => {
                draw_line(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    cx - 5.25,
                    cy,
                    cx + 5.25,
                    cy,
                );
                draw_line(
                    target,
                    stroke_style,
                    brush,
                    1.5,
                    cx,
                    cy - 5.25,
                    cx,
                    cy + 5.25,
                );
            }
            3 => draw_gear(target, stroke_style, gear_geometry, cx, cy, brush),
            _ => {
                draw_line(target, stroke_style, brush, 1.5, cx - 4.5, cy, cx + 4.5, cy);
            }
        }
    }
}

#[allow(clippy::too_many_arguments)]
unsafe fn draw_microphone(
    target: &ID2D1HwndRenderTarget,
    stroke_style: &ID2D1StrokeStyle,
    cx: f32,
    cy: f32,
    brush: &ID2D1SolidColorBrush,
    wave_primary: &ID2D1SolidColorBrush,
    wave_secondary: &ID2D1SolidColorBrush,
    state: State,
    animation_time: f32,
) {
    let body = D2D_RECT_F {
        left: cx - 2.25,
        top: cy - 6.75,
        right: cx + 2.25,
        bottom: cy + 1.5,
    };
    unsafe {
        if matches!(state, State::Recording | State::Paused) {
            target.PushAxisAlignedClip(&body, D2D1_ANTIALIAS_MODE_PER_PRIMITIVE);
            let first_drift = if state == State::Recording {
                triangle_wave(animation_time, 1.15) * 1.6
            } else {
                0.0
            };
            let second_drift = if state == State::Recording {
                triangle_wave(animation_time, 1.55) * 1.6
            } else {
                0.0
            };
            draw_mic_wave(
                target,
                stroke_style,
                wave_primary,
                cx,
                cy - 2.7,
                first_drift,
            );
            draw_mic_wave(
                target,
                stroke_style,
                wave_secondary,
                cx,
                cy - 0.75,
                second_drift,
            );
            target.PopAxisAlignedClip();
        }

        target.DrawRoundedRectangle(
            &D2D1_ROUNDED_RECT {
                rect: body,
                radiusX: 2.25,
                radiusY: 2.25,
            },
            brush,
            1.5,
            stroke_style,
        );

        let mut previous = v(cx - 5.25, cy - 0.75);
        for step in 1..=12 {
            let angle = std::f32::consts::PI - std::f32::consts::PI * step as f32 / 12.0;
            let point = v(cx + angle.cos() * 5.25, cy - 0.75 + angle.sin() * 5.25);
            target.DrawLine(previous, point, brush, 1.5, stroke_style);
            previous = point;
        }
        draw_line(
            target,
            stroke_style,
            brush,
            1.5,
            cx,
            cy + 4.5,
            cx,
            cy + 6.75,
        );
    }
}

unsafe fn draw_mic_wave(
    target: &ID2D1HwndRenderTarget,
    stroke_style: &ID2D1StrokeStyle,
    brush: &ID2D1SolidColorBrush,
    cx: f32,
    base_y: f32,
    drift: f32,
) {
    let mut previous = None;
    for step in 0..=32 {
        let x = -6.0 + step as f32 * 12.0 / 32.0 + drift;
        let phase = (x - drift + 5.25) * std::f32::consts::TAU / 4.5;
        let point = v(cx + x, base_y - phase.sin() * 0.6);
        if let Some(start) = previous {
            unsafe { target.DrawLine(start, point, brush, 1.125, stroke_style) };
        }
        previous = Some(point);
    }
}

unsafe fn draw_gear(
    target: &ID2D1HwndRenderTarget,
    stroke_style: &ID2D1StrokeStyle,
    geometry: &ID2D1PathGeometry,
    cx: f32,
    cy: f32,
    brush: &ID2D1SolidColorBrush,
) {
    unsafe {
        let mut original = Matrix3x2::default();
        target.GetTransform(&mut original);
        target.SetTransform(&Matrix3x2 {
            M11: 0.75,
            M12: 0.0,
            M21: 0.0,
            M22: 0.75,
            M31: cx - 9.0,
            M32: cy - 9.0,
        });
        target.DrawGeometry(geometry, brush, 2.0, stroke_style);
        target.SetTransform(&original);
    }
}

fn create_gear_geometry(factory: &ID2D1Factory) -> windows::core::Result<ID2D1PathGeometry> {
    unsafe {
        let geometry = factory.CreatePathGeometry()?;
        let sink = geometry.Open()?;

        sink.BeginFigure(v(12.0, 15.5), D2D1_FIGURE_BEGIN_HOLLOW);
        let mut point = v(12.0, 15.5);
        path_arc_relative(&sink, &mut point, 3.5, 3.5, true, false, 0.0, -7.0);
        path_arc_relative(&sink, &mut point, 3.5, 3.5, false, false, 0.0, 7.0);
        sink.EndFigure(D2D1_FIGURE_END_CLOSED);

        sink.BeginFigure(v(19.4, 15.0), D2D1_FIGURE_BEGIN_HOLLOW);
        point = v(19.4, 15.0);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 0.3, 1.9);
        path_line_relative(&sink, &mut point, 0.1, 0.1);
        path_arc_relative(&sink, &mut point, 2.0, 2.0, true, true, -2.8, 2.8);
        path_line_relative(&sink, &mut point, -0.1, -0.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -1.9, -0.3);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -1.0, 1.6);
        path_line_relative(&sink, &mut point, 0.0, 0.3);
        path_arc_relative(&sink, &mut point, 2.0, 2.0, true, true, -4.0, 0.0);
        path_line_absolute(&sink, &mut point, 10.0, 21.0);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -1.1, -1.6);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -1.8, 0.3);
        path_line_relative(&sink, &mut point, -0.1, 0.1);
        path_arc_absolute(&sink, &mut point, 2.0, 2.0, true, true, 4.2, 17.0);
        path_line_relative(&sink, &mut point, 0.1, -0.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 0.3, -1.9);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -1.6, -1.0);
        path_line_absolute(&sink, &mut point, 2.7, 14.0);
        path_arc_relative(&sink, &mut point, 2.0, 2.0, true, true, 0.0, -4.0);
        path_line_absolute(&sink, &mut point, 3.0, 10.0);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 1.6, -1.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -0.3, -1.8);
        path_line_absolute(&sink, &mut point, 4.2, 7.0);
        path_arc_absolute(&sink, &mut point, 2.0, 2.0, true, true, 7.0, 4.2);
        path_line_relative(&sink, &mut point, 0.1, 0.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 1.9, 0.3);
        path_line_absolute(&sink, &mut point, 9.0, 4.6);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 1.0, -1.6);
        path_line_relative(&sink, &mut point, 0.0, -0.3);
        path_arc_relative(&sink, &mut point, 2.0, 2.0, true, true, 4.0, 0.0);
        path_line_absolute(&sink, &mut point, 14.0, 3.0);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 1.1, 1.6);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 1.8, -0.3);
        path_line_relative(&sink, &mut point, 0.1, -0.1);
        path_arc_absolute(&sink, &mut point, 2.0, 2.0, true, true, 19.8, 7.0);
        path_line_relative(&sink, &mut point, -0.1, 0.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -0.3, 1.9);
        path_line_relative(&sink, &mut point, 0.0, 0.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, 1.6, 1.0);
        path_line_relative(&sink, &mut point, 0.3, 0.0);
        path_arc_relative(&sink, &mut point, 2.0, 2.0, true, true, 0.0, 4.0);
        path_line_absolute(&sink, &mut point, 21.0, 14.1);
        path_arc_relative(&sink, &mut point, 1.7, 1.7, false, false, -1.6, 0.9);
        sink.EndFigure(D2D1_FIGURE_END_CLOSED);
        sink.Close()?;
        Ok(geometry)
    }
}

unsafe fn path_line_relative(
    sink: &ID2D1GeometrySink,
    point: &mut Vector2,
    delta_x: f32,
    delta_y: f32,
) {
    point.X += delta_x;
    point.Y += delta_y;
    unsafe { sink.AddLine(*point) };
}

unsafe fn path_line_absolute(sink: &ID2D1GeometrySink, point: &mut Vector2, x: f32, y: f32) {
    *point = v(x, y);
    unsafe { sink.AddLine(*point) };
}

#[allow(clippy::too_many_arguments)]
unsafe fn path_arc_relative(
    sink: &ID2D1GeometrySink,
    point: &mut Vector2,
    radius_x: f32,
    radius_y: f32,
    large: bool,
    clockwise: bool,
    delta_x: f32,
    delta_y: f32,
) {
    let x = point.X + delta_x;
    let y = point.Y + delta_y;
    unsafe {
        path_arc_absolute(sink, point, radius_x, radius_y, large, clockwise, x, y);
    }
}

#[allow(clippy::too_many_arguments)]
unsafe fn path_arc_absolute(
    sink: &ID2D1GeometrySink,
    point: &mut Vector2,
    radius_x: f32,
    radius_y: f32,
    large: bool,
    clockwise: bool,
    x: f32,
    y: f32,
) {
    *point = v(x, y);
    unsafe {
        sink.AddArc(&D2D1_ARC_SEGMENT {
            point: *point,
            size: D2D_SIZE_F {
                width: radius_x,
                height: radius_y,
            },
            rotationAngle: 0.0,
            sweepDirection: if clockwise {
                D2D1_SWEEP_DIRECTION_CLOCKWISE
            } else {
                D2D1_SWEEP_DIRECTION_COUNTER_CLOCKWISE
            },
            arcSize: if large {
                D2D1_ARC_SIZE_LARGE
            } else {
                D2D1_ARC_SIZE_SMALL
            },
        });
    }
}

#[allow(clippy::too_many_arguments)]
unsafe fn draw_line(
    target: &ID2D1HwndRenderTarget,
    stroke_style: &ID2D1StrokeStyle,
    brush: &ID2D1SolidColorBrush,
    width: f32,
    x1: f32,
    y1: f32,
    x2: f32,
    y2: f32,
) {
    unsafe { target.DrawLine(v(x1, y1), v(x2, y2), brush, width, stroke_style) };
}

unsafe fn draw_polyline(
    target: &ID2D1HwndRenderTarget,
    stroke_style: &ID2D1StrokeStyle,
    brush: &ID2D1SolidColorBrush,
    width: f32,
    points: &[Vector2],
) {
    for pair in points.windows(2) {
        unsafe { target.DrawLine(pair[0], pair[1], brush, width, stroke_style) };
    }
}

fn triangle_wave(time: f32, duration: f32) -> f32 {
    let progress = (time / duration).rem_euclid(1.0);
    if progress < 0.5 {
        -1.0 + progress * 4.0
    } else {
        3.0 - progress * 4.0
    }
}

fn v(x: f32, y: f32) -> Vector2 {
    Vector2 { X: x, Y: y }
}

fn color(r: f32, g: f32, b: f32, a: f32) -> D2D1_COLOR_F {
    D2D1_COLOR_F { r, g, b, a }
}

fn rgb8(r: u8, g: u8, b: u8) -> D2D1_COLOR_F {
    color(r as f32 / 255.0, g as f32 / 255.0, b as f32 / 255.0, 1.0)
}

fn mix(from: D2D1_COLOR_F, to: D2D1_COLOR_F, amount: f32) -> D2D1_COLOR_F {
    let amount = amount.clamp(0.0, 1.0);
    color(
        from.r + (to.r - from.r) * amount,
        from.g + (to.g - from.g) * amount,
        from.b + (to.b - from.b) * amount,
        from.a + (to.a - from.a) * amount,
    )
}
