use std::collections::HashSet;
use std::fs;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::hotkey;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(default)]
#[allow(non_snake_case)]
pub struct Config {
    #[serde(rename = "API_ENDPOINT")]
    pub api_endpoint: String,
    #[serde(rename = "TOKEN")]
    pub token: String,
    #[serde(rename = "MODEL")]
    pub model: String,
    #[serde(rename = "LANGUAGE")]
    pub language: String,
    #[serde(rename = "PROMPT")]
    pub prompt: String,
    #[serde(rename = "TEXT_PATH")]
    pub text_path: String,
    #[serde(rename = "ExtraConfig")]
    pub extra_config: String,
    #[serde(rename = "CHANNELS")]
    pub channels: i32,
    #[serde(rename = "SAMPLING_RATE")]
    pub sampling_rate: i32,
    #[serde(rename = "SAMPLING_RATE_DEPTH")]
    pub sampling_rate_depth: i32,
    #[serde(rename = "BIT_RATE")]
    pub bit_rate: i32,
    #[serde(rename = "CODECS")]
    pub codecs: String,
    #[serde(rename = "CONTAINER")]
    pub container: String,
    #[serde(rename = "REQUEST_TIMEOUT")]
    pub request_timeout: i32,
    #[serde(rename = "MAX_RETRY")]
    pub max_retry: i32,
    #[serde(rename = "RETRY_BASE_DELAY")]
    pub retry_base_delay: f64,
    #[serde(rename = "ENABLE_HTTP2")]
    pub enable_http2: bool,
    #[serde(rename = "VERIFY_SSL")]
    pub verify_ssl: bool,
    #[serde(rename = "HOTKEY_HOOK")]
    pub hotkey_hook: bool,
    #[serde(rename = "START_KEY")]
    pub start_key: String,
    #[serde(rename = "PAUSE_KEY")]
    pub pause_key: String,
    #[serde(rename = "CANCEL_KEY")]
    pub cancel_key: String,
    #[serde(rename = "CLIPBOARD_WRITE_DELAY")]
    pub clipboard_write_delay: u64,
    #[serde(rename = "CLIPBOARD_RESTORE_DELAY")]
    pub clipboard_restore_delay: u64,
    #[serde(rename = "CACHE_DIR")]
    pub cache_dir: String,
    #[serde(rename = "KEEP_CACHE")]
    pub keep_cache: bool,
    #[serde(rename = "REQUEST_FAILED_NOTIFICATION")]
    pub request_failed_notification: bool,
    #[serde(rename = "FFMPEG_DEBUG")]
    pub ffmpeg_debug: bool,
    #[serde(rename = "RECORD_DEBUG")]
    pub record_debug: bool,
    #[serde(rename = "HOTKEY_DEBUG")]
    pub hotkey_debug: bool,
    #[serde(rename = "UPLOAD_DEBUG")]
    pub upload_debug: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            api_endpoint: String::new(),
            token: String::new(),
            model: String::new(),
            language: String::new(),
            prompt: String::new(),
            text_path: "text".into(),
            extra_config: String::new(),
            channels: 1,
            sampling_rate: 16_000,
            sampling_rate_depth: 16,
            bit_rate: 32,
            codecs: "opus".into(),
            container: "ogg".into(),
            request_timeout: 60,
            max_retry: 3,
            retry_base_delay: 0.5,
            enable_http2: true,
            verify_ssl: true,
            hotkey_hook: true,
            start_key: "ctrl+alt+q".into(),
            pause_key: "ctrl+alt+s".into(),
            cancel_key: "alt+esc".into(),
            clipboard_write_delay: 80,
            clipboard_restore_delay: 120,
            cache_dir: String::new(),
            keep_cache: false,
            request_failed_notification: false,
            ffmpeg_debug: false,
            record_debug: false,
            hotkey_debug: true,
            upload_debug: false,
        }
    }
}

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("failed to read config '{path}': {source}")]
    Read {
        path: PathBuf,
        source: std::io::Error,
    },
    #[error("failed to parse config '{path}': {source}")]
    Parse {
        path: PathBuf,
        source: serde_json::Error,
    },
    #[error("failed to serialize config: {0}")]
    Serialize(#[from] serde_json::Error),
    #[error("failed to write config '{path}': {source}")]
    Write {
        path: PathBuf,
        source: std::io::Error,
    },
    #[error("{0}")]
    Invalid(String),
}

impl Config {
    pub fn load(path: impl AsRef<Path>) -> Result<Self, ConfigError> {
        let path = path.as_ref();
        let bytes = fs::read(path).map_err(|source| ConfigError::Read {
            path: path.to_path_buf(),
            source,
        })?;
        serde_json::from_slice(&bytes).map_err(|source| ConfigError::Parse {
            path: path.to_path_buf(),
            source,
        })
    }

    pub fn save(&self, path: impl AsRef<Path>) -> Result<(), ConfigError> {
        let path = path.as_ref();
        if let Some(parent) = path
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
        {
            fs::create_dir_all(parent).map_err(|source| ConfigError::Write {
                path: path.to_path_buf(),
                source,
            })?;
        }
        let bytes = serde_json::to_vec_pretty(self)?;
        fs::write(path, bytes).map_err(|source| ConfigError::Write {
            path: path.to_path_buf(),
            source,
        })
    }

    pub fn validate(&self) -> Result<(), ConfigError> {
        if !(1..=8).contains(&self.channels) {
            return Err(ConfigError::Invalid(format!(
                "invalid Channels: {} (allowed 1..8)",
                self.channels
            )));
        }
        if self.sampling_rate <= 0 {
            return Err(ConfigError::Invalid(format!(
                "invalid SAMPLING_RATE: {} (must be > 0)",
                self.sampling_rate
            )));
        }
        if ![8, 16, 24, 32].contains(&self.sampling_rate_depth) {
            return Err(ConfigError::Invalid(format!(
                "invalid SAMPLING_RATE_DEPTH: {} (allowed: 8,16,24,32)",
                self.sampling_rate_depth
            )));
        }
        if self.bit_rate <= 0 {
            return Err(ConfigError::Invalid(format!(
                "invalid BIT_RATE: {} (must be > 0)",
                self.bit_rate
            )));
        }
        hotkey::validate_bindings(&self.start_key, &self.pause_key, &self.cancel_key)
            .map_err(|error| ConfigError::Invalid(error.to_string()))?;

        let codecs: HashSet<&str> = [
            "opus",
            "libopus",
            "wavpack",
            "aac",
            "ac3",
            "eac3",
            "mp3",
            "mp2",
            "mp1",
            "flac",
            "alac",
            "pcm",
            "vorbis",
            "libvorbis",
            "vorb",
            "adpcm",
            "amr",
            "pcm_f32be",
            "pcm_f32le",
            "pcm_f64be",
            "pcm_f64le",
            "pcm_s16be",
            "pcm_s16le",
            "pcm_s24be",
            "pcm_s24le",
            "pcm_s32be",
            "pcm_s32le",
            "pcm_s64be",
            "pcm_s64le",
            "pcm_s8",
        ]
        .into_iter()
        .collect();
        if !codecs.contains(self.codecs.to_ascii_lowercase().as_str()) {
            return Err(ConfigError::Invalid(format!(
                "invalid CODECS: {}",
                self.codecs
            )));
        }

        let containers: HashSet<&str> = [
            "wav", "ac3", "ac4", "ogg", "oga", "mp3", "flac", "eac3", "aac", "m4a", "mp4", "opus",
            "webm", "s8", "s16be", "s16le", "s24be", "s24le", "s32be", "s32le", "f32be", "f32le",
            "f64be", "f64le",
        ]
        .into_iter()
        .collect();
        if !containers.contains(self.container.to_ascii_lowercase().as_str()) {
            return Err(ConfigError::Invalid(format!(
                "invalid CONTAINER: {}",
                self.container
            )));
        }
        Ok(())
    }

    pub fn container_extension(&self) -> String {
        container_extension(&self.container)
    }
}

pub fn container_extension(container: &str) -> String {
    let lower = container.to_ascii_lowercase();
    if lower.is_empty() {
        "ogg".into()
    } else {
        lower
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn old_notification_is_ignored_and_not_written_back() {
        let input = r#"{"NOTIFICATION":true,"MODEL":"whisper"}"#;
        let cfg: Config = serde_json::from_str(input).unwrap();
        assert_eq!(cfg.model, "whisper");
        let output: serde_json::Map<String, serde_json::Value> =
            serde_json::from_str(&serde_json::to_string(&cfg).unwrap()).unwrap();
        assert!(!output.contains_key("NOTIFICATION"));
        assert!(output.contains_key("REQUEST_FAILED_NOTIFICATION"));
    }

    #[test]
    fn missing_and_unknown_fields_are_compatible() {
        let cfg: Config = serde_json::from_str(r#"{"UNKNOWN":1,"CHANNELS":2}"#).unwrap();
        assert_eq!(cfg.channels, 2);
        assert_eq!(cfg.text_path, "text");
        assert_eq!(cfg.codecs, "opus");
        assert_eq!(cfg.clipboard_write_delay, 80);
        assert_eq!(cfg.clipboard_restore_delay, 120);
    }

    #[test]
    fn validates_equivalent_hotkeys() {
        let cfg = Config {
            pause_key: "ALT + CONTROL + Q".into(),
            ..Config::default()
        };
        assert!(
            cfg.validate()
                .unwrap_err()
                .to_string()
                .contains("duplicates START_KEY")
        );
    }

    #[test]
    fn validates_audio_ranges_and_case_insensitive_formats() {
        let mut cfg = Config {
            codecs: "MP3".into(),
            container: "M4A".into(),
            ..Config::default()
        };
        cfg.validate().unwrap();
        cfg.channels = 0;
        assert!(cfg.validate().is_err());
    }

    #[test]
    fn round_trip_default_has_no_removed_field() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("config.json");
        Config::default().save(&path).unwrap();
        assert_eq!(Config::load(&path).unwrap(), Config::default());
        let raw: serde_json::Map<String, serde_json::Value> =
            serde_json::from_str(&fs::read_to_string(path).unwrap()).unwrap();
        assert!(!raw.contains_key("NOTIFICATION"));
    }
}
