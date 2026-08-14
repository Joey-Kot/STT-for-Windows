use std::collections::BTreeMap;
use std::path::Path;
use std::time::Duration;

use reqwest::multipart::{Form, Part};
use reqwest::{Client, StatusCode};
use serde_json::Value;
use thiserror::Error;
use tokio::fs::File;
use tokio_util::io::ReaderStream;
use tokio_util::sync::CancellationToken;

use crate::Config;
use crate::jsonpath;

const USER_AGENT: &str = "stt-go-client/1.0";

#[derive(Debug, Clone)]
pub struct Transcription {
    pub text: String,
    pub raw_response: Vec<u8>,
}

#[derive(Debug, Error)]
pub enum AsrError {
    #[error("API endpoint is empty")]
    EmptyEndpoint,
    #[error("invalid extra-config JSON: {0}")]
    InvalidExtraConfig(#[from] serde_json::Error),
    #[error("failed to build HTTP client: {0}")]
    Client(#[source] reqwest::Error),
    #[error("{0}")]
    Attempt(String),
    #[error("request canceled")]
    Canceled,
    #[error("exceeded max retries ({max_retry}), attempts: {attempts}")]
    RetryExhausted {
        max_retry: i32,
        attempts: i32,
        last_response: Vec<u8>,
    },
}

impl AsrError {
    pub fn is_retry_exhausted(&self) -> bool {
        matches!(self, Self::RetryExhausted { .. })
    }

    pub fn last_response(&self) -> &[u8] {
        match self {
            Self::RetryExhausted { last_response, .. } => last_response,
            _ => &[],
        }
    }
}

#[derive(Clone)]
pub struct AsrClient {
    config: Config,
    client: Client,
    extra_config: Option<BTreeMap<String, Value>>,
}

impl AsrClient {
    pub fn new(config: Config) -> Result<Self, AsrError> {
        let extra_config = if config.extra_config.is_empty() {
            None
        } else {
            Some(serde_json::from_str::<BTreeMap<String, Value>>(
                &config.extra_config,
            )?)
        };
        let mut builder = Client::builder()
            .danger_accept_invalid_certs(!config.verify_ssl)
            .redirect(reqwest::redirect::Policy::none())
            .no_proxy()
            .no_gzip()
            .no_brotli()
            .no_deflate();
        if config.request_timeout > 0 {
            builder = builder.timeout(Duration::from_secs(config.request_timeout as u64));
        }
        if !config.enable_http2 {
            builder = builder.http1_only();
        }
        let client = builder.build().map_err(AsrError::Client)?;
        Ok(Self {
            config,
            client,
            extra_config,
        })
    }

    pub async fn transcribe(
        &self,
        cancellation: &CancellationToken,
        file_path: &Path,
    ) -> Result<Transcription, AsrError> {
        if cancellation.is_cancelled() {
            return Err(AsrError::Canceled);
        }
        if self.config.api_endpoint.is_empty() {
            return Err(AsrError::EmptyEndpoint);
        }

        let mut attempt = 0;
        let mut delay = self.config.retry_base_delay;
        loop {
            attempt += 1;
            let (succeeded, response) = self.upload_once(cancellation, file_path).await?;
            if succeeded {
                return Ok(Transcription {
                    text: jsonpath::extract_text_from_response(&response, &self.config.text_path),
                    raw_response: response,
                });
            }
            if self.config.upload_debug {
                eprintln!(
                    "[upload] attempt {attempt} failed: {}",
                    format_response(&response)
                );
            }
            if attempt >= self.config.max_retry {
                return Err(AsrError::RetryExhausted {
                    max_retry: self.config.max_retry,
                    attempts: attempt,
                    last_response: response,
                });
            }
            let duration = Duration::from_secs_f64(delay.max(0.0));
            tokio::select! {
                _ = cancellation.cancelled() => return Err(AsrError::Canceled),
                _ = tokio::time::sleep(duration) => {}
            }
            delay *= 2.0;
        }
    }

    async fn upload_once(
        &self,
        cancellation: &CancellationToken,
        file_path: &Path,
    ) -> Result<(bool, Vec<u8>), AsrError> {
        if self.config.upload_debug {
            eprintln!(
                "[upload] uploading {} -> {}",
                file_path.display(),
                self.config.api_endpoint
            );
        }
        let response = match self.send_request(cancellation, file_path).await {
            Ok(response) => response,
            Err(AsrError::Canceled) => return Err(AsrError::Canceled),
            Err(AsrError::Attempt(message)) => return Ok((false, message.into_bytes())),
            Err(error) => return Ok((false, format!("request error: {error}").into_bytes())),
        };
        let status = response.status();
        let bytes = tokio::select! {
            _ = cancellation.cancelled() => return Err(AsrError::Canceled),
            result = response.bytes() => match result {
                Ok(bytes) => bytes.to_vec(),
                Err(error) => format!("read response error: {error}").into_bytes(),
            }
        };
        Ok((status == StatusCode::OK, bytes))
    }

    async fn send_request(
        &self,
        cancellation: &CancellationToken,
        file_path: &Path,
    ) -> Result<reqwest::Response, AsrError> {
        // The file and multipart body are reconstructed for every retry.
        let file = File::open(file_path)
            .await
            .map_err(|error| AsrError::Attempt(format!("open file error: {error}")))?;
        let file_name = file_path
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_else(|| "audio".into());
        let body = reqwest::Body::wrap_stream(ReaderStream::new(file));
        let part = Part::stream(body).file_name(file_name);
        let mut form = Form::new().part("file", part);
        for (key, value) in self.request_fields() {
            form = form.text(key, value);
        }

        let mut request = self
            .client
            .post(&self.config.api_endpoint)
            .header(reqwest::header::USER_AGENT, USER_AGENT)
            .multipart(form);
        if !self.config.token.is_empty() {
            request = request.bearer_auth(&self.config.token);
        }
        tokio::select! {
            _ = cancellation.cancelled() => Err(AsrError::Canceled),
            result = request.send() => result.map_err(AsrError::Client),
        }
    }

    pub fn request_fields(&self) -> BTreeMap<String, String> {
        let mut fields = BTreeMap::new();
        if !self.config.model.is_empty() {
            fields.insert("model".into(), self.config.model.clone());
        }
        if !self.config.language.is_empty() {
            fields.insert("language".into(), self.config.language.clone());
        }
        if !self.config.prompt.is_empty() {
            fields.insert("prompt".into(), self.config.prompt.clone());
        }
        if let Some(extra) = &self.extra_config {
            for (key, value) in extra {
                if value.is_null() {
                    fields.remove(key);
                } else {
                    fields.insert(key.clone(), value_to_form_field(value));
                }
            }
        }
        fields
    }
}

fn value_to_form_field(value: &Value) -> String {
    match value {
        Value::String(value) => value.clone(),
        Value::Bool(value) => value.to_string(),
        Value::Number(value) => value.to_string(),
        _ => serde_json::to_string(value).unwrap_or_else(|_| value.to_string()),
    }
}

pub fn format_response(response: &[u8]) -> String {
    if response.is_empty() {
        return "<empty>".into();
    }
    if let Ok(text) = std::str::from_utf8(response) {
        if text.len() > 1_000 {
            let end = text
                .char_indices()
                .map(|(index, _)| index)
                .take_while(|index| *index <= 1_000)
                .last()
                .unwrap_or(0);
            return format!(
                "{}... (truncated, total {} bytes)",
                &text[..end],
                response.len()
            );
        }
        return text.into();
    }
    let prefix = &response[..response.len().min(256)];
    let hex: String = prefix.iter().map(|byte| format!("{byte:02x}")).collect();
    if response.len() > 256 {
        format!("<binary {} bytes, prefix hex: {hex}...>", response.len())
    } else {
        format!("<binary {} bytes, hex: {hex}>", response.len())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extra_config_shallowly_overrides_and_null_deletes() {
        let cfg = Config {
            model: "base".into(),
            language: "zh".into(),
            prompt: "hello".into(),
            extra_config:
                r#"{"language":null,"temperature":0.25,"stream":false,"metadata":{"tier":"test"}}"#
                    .into(),
            ..Config::default()
        };
        let fields = AsrClient::new(cfg).unwrap().request_fields();
        assert!(!fields.contains_key("language"));
        assert_eq!(fields["model"], "base");
        assert_eq!(fields["temperature"], "0.25");
        assert_eq!(fields["stream"], "false");
        assert_eq!(fields["metadata"], r#"{"tier":"test"}"#);
    }

    #[test]
    fn rejects_non_object_extra_config() {
        let config = Config {
            extra_config: "[]".into(),
            ..Config::default()
        };
        assert!(AsrClient::new(config).is_err());
    }

    #[test]
    fn formats_text_and_binary_responses() {
        assert_eq!(format_response(&[]), "<empty>");
        assert_eq!(format_response(b"hello"), "hello");
        assert_eq!(format_response(&[0xff, 0]), "<binary 2 bytes, hex: ff00>");
        let unicode = format!("{}é", "a".repeat(999));
        assert!(format_response(unicode.as_bytes()).starts_with(&"a".repeat(999)));
    }
}
