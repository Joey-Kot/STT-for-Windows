use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::Arc;

use async_trait::async_trait;
use clap::{ArgAction, Parser};
use stt_core::Config;
use stt_core::converter::{AudioConverter, ConvertError, ffmpeg_args, paths_equal, settings_for};
use stt_core::runtime::{Runtime, run_file_mode};
use tokio::process::Command;
use tokio_util::sync::CancellationToken;

#[derive(Debug, Default)]
struct ExternalFfmpegConverter;

#[async_trait]
impl AudioConverter for ExternalFfmpegConverter {
    async fn convert(
        &self,
        cancellation: &CancellationToken,
        config: &Config,
        input: &Path,
        output: &Path,
        source_rate: i32,
    ) -> Result<(), ConvertError> {
        if cancellation.is_cancelled() {
            return Err(ConvertError::Canceled);
        }
        if paths_equal(input, output) {
            return Err(ConvertError::SamePath);
        }
        let settings = settings_for(config, source_rate)?;
        let arguments = ffmpeg_args(&settings, input, output);
        if config.ffmpeg_debug {
            eprintln!("[ffmpeg] executing: ffmpeg {}", arguments.join(" "));
        }
        #[cfg(windows)]
        let executable = "ffmpeg.exe";
        #[cfg(not(windows))]
        let executable = "ffmpeg";
        let mut command = Command::new(executable);
        command
            .args(&arguments)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .kill_on_drop(true);
        let future = command.output();
        tokio::pin!(future);
        let result = tokio::select! {
            _ = cancellation.cancelled() => return Err(ConvertError::Canceled),
            result = &mut future => result.map_err(ConvertError::Start)?,
        };
        if !result.status.success() {
            return Err(ConvertError::Failed {
                message: String::from_utf8_lossy(&result.stderr).into_owned(),
            });
        }
        Ok(())
    }
}

#[derive(Debug, Parser)]
#[command(
    name = "stt",
    version,
    about = "Record audio, transcribe it through an ASR API, and paste the result.",
    disable_help_flag = false
)]
struct Arguments {
    /// Path to a JSON configuration file.
    #[arg(long, value_name = "PATH", help_heading = "General")]
    config: Option<PathBuf>,

    /// Existing audio file to transcode and transcribe.
    #[arg(long, value_name = "PATH", help_heading = "General")]
    file: Option<PathBuf>,

    /// Output text path for --file mode.
    #[arg(long, value_name = "PATH", help_heading = "General")]
    output: Option<PathBuf>,

    /// ASR HTTP endpoint URL.
    #[arg(long, value_name = "URL", help_heading = "API")]
    api_endpoint: Option<String>,
    /// Bearer token sent with the ASR request.
    #[arg(long, value_name = "TOKEN", help_heading = "API")]
    token: Option<String>,
    /// Model multipart field.
    #[arg(long, value_name = "MODEL", help_heading = "API")]
    model: Option<String>,
    /// Language multipart field.
    #[arg(long, value_name = "LANGUAGE", help_heading = "API")]
    language: Option<String>,
    /// Prompt multipart field.
    #[arg(long, value_name = "TEXT", help_heading = "API")]
    prompt: Option<String>,
    /// Dot path used to extract text from the JSON response.
    #[arg(long, value_name = "PATH", help_heading = "API")]
    text_path: Option<String>,
    /// Stringified JSON object with extra multipart fields.
    #[arg(long, value_name = "JSON", help_heading = "API")]
    extra_config: Option<String>,

    /// Audio encoder or compatible alias.
    #[arg(long, value_name = "CODEC", help_heading = "Audio")]
    codecs: Option<String>,
    /// Output audio container.
    #[arg(long, value_name = "FORMAT", help_heading = "Audio")]
    container: Option<String>,
    /// Recording and conversion channel count.
    #[arg(long, value_name = "N", help_heading = "Audio")]
    channels: Option<i32>,
    /// Recording and conversion sample rate in Hz.
    #[arg(long, alias = "rate", value_name = "HZ", help_heading = "Audio")]
    sampling_rate: Option<i32>,
    /// Conversion sample depth in bits.
    #[arg(long, value_name = "BITS", help_heading = "Audio")]
    sampling_rate_depth: Option<i32>,
    /// Audio bitrate in kbps.
    #[arg(long, value_name = "KBPS", help_heading = "Audio")]
    bit_rate: Option<i32>,

    /// Per-request client timeout in seconds.
    #[arg(long, value_name = "SECONDS", help_heading = "Network")]
    request_timeout: Option<i32>,
    /// Maximum request attempts, including the first request.
    #[arg(long, value_name = "N", help_heading = "Network")]
    max_retry: Option<i32>,
    /// Initial exponential-backoff delay in seconds.
    #[arg(long, value_name = "SECONDS", help_heading = "Network")]
    retry_base_delay: Option<f64>,
    /// Enable HTTP/2 negotiation.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Network"
    )]
    enable_http2: Option<bool>,
    /// Verify TLS certificates.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Network"
    )]
    verify_ssl: Option<bool>,

    /// Start/stop recording hotkey.
    #[arg(long, value_name = "HOTKEY", help_heading = "Hotkeys")]
    start_key: Option<String>,
    /// Pause/resume recording hotkey.
    #[arg(long, value_name = "HOTKEY", help_heading = "Hotkeys")]
    pause_key: Option<String>,
    /// Recording/request cancellation hotkey.
    #[arg(long, value_name = "HOTKEY", help_heading = "Hotkeys")]
    cancel_key: Option<String>,
    /// Use the low-level keyboard hook instead of RegisterHotKey.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Hotkeys"
    )]
    hotkey_hook: Option<bool>,
    /// Milliseconds to wait after writing the transcription before sending Ctrl+V.
    #[arg(long, value_name = "MS", help_heading = "Hotkeys")]
    clipboard_write_delay: Option<u64>,
    /// Milliseconds to wait after Ctrl+V before restoring the original clipboard text.
    #[arg(long, value_name = "MS", help_heading = "Hotkeys")]
    clipboard_restore_delay: Option<u64>,

    /// Directory used for temporary and retained cache files.
    #[arg(long, value_name = "PATH", help_heading = "Cache")]
    cache_dir: Option<String>,
    /// Retain audio and successful response cache files.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Cache"
    )]
    keep_cache: Option<bool>,
    /// Paste [request failed] after all request attempts fail.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Cache"
    )]
    request_failed_notification: Option<bool>,

    /// Enable FFmpeg conversion diagnostics.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Debug"
    )]
    ffmpeg_debug: Option<bool>,
    /// Enable recording diagnostics.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Debug"
    )]
    record_debug: Option<bool>,
    /// Enable hotkey diagnostics.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Debug"
    )]
    hotkey_debug: Option<bool>,
    /// Enable ASR upload diagnostics.
    #[arg(
        long,
        value_name = "BOOL",
        action = ArgAction::Set,
        help_heading = "Debug"
    )]
    upload_debug: Option<bool>,
}

impl Arguments {
    fn has_config_override(&self) -> bool {
        self.api_endpoint.is_some()
            || self.token.is_some()
            || self.model.is_some()
            || self.language.is_some()
            || self.prompt.is_some()
            || self.text_path.is_some()
            || self.extra_config.is_some()
            || self.codecs.is_some()
            || self.container.is_some()
            || self.channels.is_some()
            || self.sampling_rate.is_some()
            || self.sampling_rate_depth.is_some()
            || self.bit_rate.is_some()
            || self.request_timeout.is_some()
            || self.max_retry.is_some()
            || self.retry_base_delay.is_some()
            || self.enable_http2.is_some()
            || self.verify_ssl.is_some()
            || self.start_key.is_some()
            || self.pause_key.is_some()
            || self.cancel_key.is_some()
            || self.hotkey_hook.is_some()
            || self.clipboard_write_delay.is_some()
            || self.clipboard_restore_delay.is_some()
            || self.cache_dir.is_some()
            || self.keep_cache.is_some()
            || self.request_failed_notification.is_some()
            || self.ffmpeg_debug.is_some()
            || self.record_debug.is_some()
            || self.hotkey_debug.is_some()
            || self.upload_debug.is_some()
            || self.output.is_some()
    }

    fn apply(self, config: &mut Config) {
        set(&mut config.api_endpoint, self.api_endpoint);
        set(&mut config.token, self.token);
        set(&mut config.model, self.model);
        set(&mut config.language, self.language);
        set(&mut config.prompt, self.prompt);
        set(&mut config.text_path, self.text_path);
        set(&mut config.extra_config, self.extra_config);
        set(&mut config.codecs, self.codecs);
        set(&mut config.container, self.container);
        set(&mut config.channels, self.channels);
        set(&mut config.sampling_rate, self.sampling_rate);
        set(&mut config.sampling_rate_depth, self.sampling_rate_depth);
        set(&mut config.bit_rate, self.bit_rate);
        set(&mut config.request_timeout, self.request_timeout);
        set(&mut config.max_retry, self.max_retry);
        set(&mut config.retry_base_delay, self.retry_base_delay);
        set(&mut config.enable_http2, self.enable_http2);
        set(&mut config.verify_ssl, self.verify_ssl);
        set(&mut config.start_key, self.start_key);
        set(&mut config.pause_key, self.pause_key);
        set(&mut config.cancel_key, self.cancel_key);
        set(&mut config.hotkey_hook, self.hotkey_hook);
        set(
            &mut config.clipboard_write_delay,
            self.clipboard_write_delay,
        );
        set(
            &mut config.clipboard_restore_delay,
            self.clipboard_restore_delay,
        );
        set(&mut config.cache_dir, self.cache_dir);
        set(&mut config.keep_cache, self.keep_cache);
        set(
            &mut config.request_failed_notification,
            self.request_failed_notification,
        );
        set(&mut config.ffmpeg_debug, self.ffmpeg_debug);
        set(&mut config.record_debug, self.record_debug);
        set(&mut config.hotkey_debug, self.hotkey_debug);
        set(&mut config.upload_debug, self.upload_debug);
    }
}

fn set<T>(target: &mut T, value: Option<T>) {
    if let Some(value) = value {
        *target = value;
    }
}

#[tokio::main]
async fn main() {
    if let Err(error) = run().await {
        eprintln!("[main] {error}");
        std::process::exit(1);
    }
}

async fn run() -> Result<(), Box<dyn std::error::Error>> {
    let arguments = Arguments::parse();
    let explicit_config = arguments.config.clone();
    let file = arguments.file.clone();
    let output = arguments.output.clone();
    let has_override = arguments.has_config_override();

    let mut config = if let Some(path) = explicit_config {
        Config::load(path)?
    } else if Path::new("config.json").exists() {
        Config::load("config.json")?
    } else if !has_override {
        Config::default().save("config.json")?;
        let path = std::env::current_dir()?.join("config.json");
        println!(
            "[main] default config created at {}. Please edit it and re-run.",
            path.display()
        );
        return Ok(());
    } else {
        Config::default()
    };

    arguments.apply(&mut config);
    config.validate()?;
    let converter = Arc::new(ExternalFfmpegConverter);

    if let Some(file) = file {
        let path = run_file_mode(config, converter, &file, output.as_deref()).await?;
        println!("[main] transcription written to {}", path.display());
        return Ok(());
    }

    let runtime = Runtime::new(config, converter)?;
    runtime.set_event_handler(Some(Arc::new(|event| {
        if event.error.is_empty() {
            println!("[state] {}: {}", event.state, event.message);
        } else {
            println!(
                "[state] {}: {} ({})",
                event.state, event.message, event.error
            );
        }
    })));
    runtime.start_hotkeys()?;
    println!("[main] ready. Use hotkeys to start/stop/pause/cancel.");
    tokio::signal::ctrl_c().await?;
    runtime.stop();
    Ok(())
}

#[cfg(test)]
mod tests {
    use clap::{CommandFactory, Parser};

    use super::*;

    #[test]
    fn standard_boolean_values_and_alias_apply() {
        let arguments = Arguments::try_parse_from([
            "stt",
            "--verify-ssl",
            "false",
            "--enable-http2",
            "true",
            "--hotkey-hook",
            "false",
            "--clipboard-write-delay",
            "25",
            "--clipboard-restore-delay",
            "75",
            "--rate",
            "22050",
        ])
        .unwrap();
        let mut config = Config::default();
        arguments.apply(&mut config);
        assert!(!config.verify_ssl);
        assert!(config.enable_http2);
        assert!(!config.hotkey_hook);
        assert_eq!(config.clipboard_write_delay, 25);
        assert_eq!(config.clipboard_restore_delay, 75);
        assert_eq!(config.sampling_rate, 22_050);
    }

    #[test]
    fn help_is_grouped_like_gui_settings() {
        let mut command = Arguments::command();
        let help = command.render_long_help().to_string();
        let mut previous = 0;
        for heading in [
            "General:", "API:", "Audio:", "Network:", "Hotkeys:", "Cache:", "Debug:",
        ] {
            let position = help
                .find(heading)
                .unwrap_or_else(|| panic!("missing help heading {heading:?}\n{help}"));
            assert!(
                position >= previous,
                "help heading order is incorrect\n{help}"
            );
            previous = position;
        }
    }

    #[test]
    fn removed_notification_argument_is_rejected() {
        let error = Arguments::try_parse_from(["stt", "--notification", "true"]).unwrap_err();
        assert!(
            error
                .to_string()
                .contains("unexpected argument '--notification'")
        );
    }

    #[test]
    fn single_dash_legacy_argument_is_rejected() {
        assert!(Arguments::try_parse_from(["stt", "-token", "secret"]).is_err());
    }
}
