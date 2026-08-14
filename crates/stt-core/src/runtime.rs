use std::path::{Path, PathBuf};
use std::sync::{Arc, Weak};
use std::time::{Duration, Instant};

use parking_lot::Mutex;
use serde::{Deserialize, Serialize};
use thiserror::Error;
use tokio::runtime::Handle;
use tokio::sync::{Mutex as AsyncMutex, OwnedMutexGuard};
use tokio_util::sync::CancellationToken;

use crate::Config;
use crate::asr::{AsrClient, AsrError, Transcription};
use crate::cache;
use crate::clipboard::{self, ClipboardError};
use crate::converter::{AudioConverter, ConvertError};
use crate::hotkey::{self, HotkeyRegistration};
use crate::recorder::{Recorder, RecorderError, RecorderState, RecordingResult};

const SHUTDOWN_GRACE_PERIOD: Duration = Duration::from_millis(250);
const SHUTDOWN_POLL_INTERVAL: Duration = Duration::from_millis(5);

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum State {
    Idle,
    Recording,
    Paused,
    Uploading,
    Error,
}

impl std::fmt::Display for State {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(formatter, "{self:?}")
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Event {
    pub state: State,
    pub message: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub error: String,
}

impl Default for Event {
    fn default() -> Self {
        Self {
            state: State::Idle,
            message: String::new(),
            error: String::new(),
        }
    }
}

#[derive(Debug, Error)]
pub enum RuntimeError {
    #[error("{0}")]
    Config(#[from] crate::config::ConfigError),
    #[error("{0}")]
    Asr(#[from] AsrError),
    #[error("{0}")]
    Recorder(#[from] RecorderError),
    #[error("{0}")]
    Convert(#[from] ConvertError),
    #[error("{0}")]
    Clipboard(#[from] ClipboardError),
    #[error("{0}")]
    Hotkey(#[from] hotkey::HotkeyError),
    #[error("runtime stopped")]
    Stopped,
    #[error("runtime action is busy")]
    Busy,
    #[error("cannot save settings while {0}")]
    CannotReload(State),
    #[error("recording completed without a WAV path")]
    MissingWav,
    #[error("input file '{path}' is unavailable: {source}")]
    InputFile {
        path: PathBuf,
        source: std::io::Error,
    },
    #[error("failed to write transcription '{path}': {source}")]
    OutputFile {
        path: PathBuf,
        source: std::io::Error,
    },
}

struct RuntimeInner {
    config: Config,
    temp_dir: PathBuf,
    recorder: Arc<Recorder>,
    asr_client: Arc<AsrClient>,
    hotkeys: Option<HotkeyRegistration>,
    event: Event,
    event_handler: Option<Arc<dyn Fn(Event) + Send + Sync>>,
    next_session: u64,
    active_session: u64,
    active_request_cancellation: Option<CancellationToken>,
}

pub struct Runtime {
    inner: Mutex<RuntimeInner>,
    action_lock: Arc<AsyncMutex<()>>,
    lifecycle: CancellationToken,
    converter: Arc<dyn AudioConverter>,
    executor: Handle,
}

impl std::fmt::Debug for Runtime {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("Runtime")
            .field("event", &self.snapshot())
            .finish_non_exhaustive()
    }
}

impl Runtime {
    pub fn new(
        mut config: Config,
        converter: Arc<dyn AudioConverter>,
    ) -> Result<Arc<Self>, RuntimeError> {
        config.validate()?;
        let temp_dir = cache::initialize_cache_dir(&mut config);
        cache::cleanup_old_temp_items(&temp_dir);
        let asr_client = Arc::new(AsrClient::new(config.clone())?);
        let recorder = Arc::new(Recorder::new(config.clone(), temp_dir.clone()));
        let runtime = Arc::new(Self {
            inner: Mutex::new(RuntimeInner {
                config,
                temp_dir,
                recorder,
                asr_client,
                hotkeys: None,
                event: Event::default(),
                event_handler: None,
                next_session: 0,
                active_session: 0,
                active_request_cancellation: None,
            }),
            action_lock: Arc::new(AsyncMutex::new(())),
            lifecycle: CancellationToken::new(),
            converter,
            executor: Handle::current(),
        });
        Ok(runtime)
    }

    pub fn set_event_handler(&self, handler: Option<Arc<dyn Fn(Event) + Send + Sync>>) {
        self.inner.lock().event_handler = handler;
    }

    pub fn snapshot(&self) -> Event {
        self.inner.lock().event.clone()
    }

    pub fn config(&self) -> Config {
        self.inner.lock().config.clone()
    }

    pub fn can_reload(&self) -> bool {
        matches!(self.snapshot().state, State::Idle | State::Error)
    }

    pub fn is_stopped(&self) -> bool {
        self.lifecycle.is_cancelled()
    }

    pub fn try_toggle_recording(self: &Arc<Self>) -> bool {
        self.try_action(1)
    }

    pub fn try_toggle_pause(self: &Arc<Self>) -> bool {
        self.try_action(2)
    }

    pub fn try_cancel(self: &Arc<Self>) -> bool {
        self.try_action(3)
    }

    pub async fn handle_action(self: &Arc<Self>, id: i32) -> bool {
        if self.is_stopped() {
            return false;
        }
        if id == 3 && self.cancel_active_request() {
            return true;
        }
        let Ok(guard) = self.action_lock.clone().try_lock_owned() else {
            return false;
        };
        self.clone().handle_action_with_guard(id, guard).await;
        true
    }

    fn try_action(self: &Arc<Self>, id: i32) -> bool {
        if self.is_stopped() {
            return false;
        }
        if id == 3 && self.cancel_active_request() {
            return true;
        }
        let Ok(guard) = self.action_lock.clone().try_lock_owned() else {
            return false;
        };
        let runtime = self.clone();
        self.executor.spawn(async move {
            runtime.handle_action_with_guard(id, guard).await;
        });
        true
    }

    async fn handle_action_with_guard(self: Arc<Self>, id: i32, _guard: OwnedMutexGuard<()>) {
        if self.is_stopped() {
            return;
        }
        match id {
            1 => self.toggle_recording_locked().await,
            2 => self.toggle_pause_locked(),
            3 => self.cancel_recording_locked().await,
            _ => {}
        }
    }

    pub fn start_hotkeys(self: &Arc<Self>) -> Result<(), RuntimeError> {
        if self.is_stopped() {
            return Err(RuntimeError::Stopped);
        }
        let config = {
            let inner = self.inner.lock();
            if inner.hotkeys.is_some() {
                return Ok(());
            }
            inner.config.clone()
        };
        let weak = Arc::downgrade(self);
        let registration = hotkey::register(
            &config.start_key,
            &config.pause_key,
            &config.cancel_key,
            config.hotkey_hook,
            move |id| {
                if let Some(runtime) = weak.upgrade()
                    && !runtime.try_action(id)
                    && runtime.config().hotkey_debug
                {
                    eprintln!(
                        "[hotkey-debug] dropped action id={id} while another action is in progress"
                    );
                }
            },
            config.hotkey_debug,
        );
        match registration {
            Ok(registration) => {
                if self.is_stopped() {
                    registration.stop();
                    return Err(RuntimeError::Stopped);
                }
                self.inner.lock().hotkeys = Some(registration);
                Ok(())
            }
            Err(error) => {
                self.set_state(State::Error, "Hotkey registration failed", Some(&error));
                Err(error.into())
            }
        }
    }

    pub async fn reload(self: &Arc<Self>, mut config: Config) -> Result<(), RuntimeError> {
        if self.is_stopped() {
            return Err(RuntimeError::Stopped);
        }
        let _guard = self
            .action_lock
            .clone()
            .try_lock_owned()
            .map_err(|_| RuntimeError::Busy)?;
        let current_state = self.snapshot().state;
        if !matches!(current_state, State::Idle | State::Error) {
            return Err(RuntimeError::CannotReload(current_state));
        }
        config.validate()?;
        let temp_dir = cache::initialize_cache_dir(&mut config);
        let asr_client = Arc::new(AsrClient::new(config.clone())?);
        let recorder = Arc::new(Recorder::new(config.clone(), temp_dir.clone()));
        if let Some(previous_hotkeys) = self.inner.lock().hotkeys.take() {
            previous_hotkeys.stop_and_wait();
        }
        {
            let mut inner = self.inner.lock();
            inner.config = config;
            inner.temp_dir = temp_dir;
            inner.asr_client = asr_client;
            inner.recorder = recorder;
            inner.active_session = 0;
            inner.active_request_cancellation = None;
        }
        // start_hotkeys commits a stable Error event before returning an error.
        self.start_hotkeys()?;
        self.set_state(State::Idle, "Settings saved", None::<&RuntimeError>);
        Ok(())
    }

    pub fn stop(&self) {
        if self.lifecycle.is_cancelled() {
            return;
        }
        self.lifecycle.cancel();
        let (recorder, hotkeys, request_cancellation) = {
            let mut inner = self.inner.lock();
            (
                inner.recorder.clone(),
                inner.hotkeys.take(),
                inner.active_request_cancellation.take(),
            )
        };
        recorder.request_cancel();
        if let Some(cancellation) = request_cancellation {
            cancellation.cancel();
        }
        if let Some(hotkeys) = hotkeys {
            hotkeys.stop();
        }

        let deadline = Instant::now() + SHUTDOWN_GRACE_PERIOD;
        while Instant::now() < deadline {
            if let Ok(guard) = self.action_lock.try_lock() {
                drop(guard);
                return;
            }
            std::thread::sleep(SHUTDOWN_POLL_INTERVAL);
        }
    }

    async fn toggle_recording_locked(self: &Arc<Self>) {
        let state = self.snapshot().state;
        if matches!(state, State::Idle | State::Error) {
            let (recorder, session) = self.begin_recording_session();
            if let Err(error) = recorder.start(self.lifecycle.child_token()).await {
                self.clear_recording_session(&recorder, session);
                if !self.is_stopped() {
                    self.set_state(State::Error, "Recording start failed", Some(&error));
                }
                return;
            }
            if self.is_stopped() {
                recorder.request_cancel();
                self.clear_recording_session(&recorder, session);
                return;
            }
            self.set_state(State::Recording, "Recording started", None::<&RuntimeError>);
            return;
        }
        if !matches!(state, State::Recording | State::Paused) {
            return;
        }
        let (recorder, session) = {
            let inner = self.inner.lock();
            (inner.recorder.clone(), inner.active_session)
        };
        match recorder.stop().await {
            Ok(result) => {
                self.clear_recording_session(&recorder, session);
                if self.is_stopped() {
                    discard_recording(&result);
                    return;
                }
                if result.canceled {
                    self.set_state(State::Idle, "Recording canceled", None::<&RuntimeError>);
                    return;
                }
                let cancellation = self.begin_active_request();
                self.set_state(
                    State::Uploading,
                    "Uploading ASR request",
                    None::<&RuntimeError>,
                );
                self.transcribe_recording(result, &cancellation).await;
                self.clear_active_request();
            }
            Err(error) => {
                if !matches!(
                    error,
                    RecorderError::NotRunning | RecorderError::WorkerStopped
                ) {
                    self.clear_recording_session(&recorder, session);
                }
                if !self.is_stopped() {
                    self.set_state(State::Error, "Recording stop failed", Some(&error));
                }
            }
        }
    }

    fn begin_recording_session(self: &Arc<Self>) -> (Arc<Recorder>, u64) {
        let (recorder, session) = {
            let mut inner = self.inner.lock();
            inner.next_session += 1;
            inner.active_session = inner.next_session;
            (inner.recorder.clone(), inner.active_session)
        };
        let weak: Weak<Self> = Arc::downgrade(self);
        let recorder_for_callback = Arc::downgrade(&recorder);
        recorder.set_error_handler(Some(Arc::new(move |error| {
            let Some(runtime) = weak.upgrade() else {
                return;
            };
            let Some(recorder) = recorder_for_callback.upgrade() else {
                return;
            };
            let executor = runtime.executor.clone();
            executor.spawn(async move {
                let _guard = runtime.action_lock.lock().await;
                runtime.handle_recorder_error(&recorder, session, error);
            });
        })));
        (recorder, session)
    }

    fn clear_recording_session(&self, recorder: &Arc<Recorder>, session: u64) {
        let mut inner = self.inner.lock();
        if Arc::ptr_eq(&inner.recorder, recorder) && inner.active_session == session {
            inner.active_session = 0;
        }
    }

    fn begin_active_request(&self) -> CancellationToken {
        let cancellation = self.lifecycle.child_token();
        self.inner.lock().active_request_cancellation = Some(cancellation.clone());
        cancellation
    }

    fn clear_active_request(&self) {
        self.inner.lock().active_request_cancellation = None;
    }

    fn cancel_active_request(&self) -> bool {
        let cancellation = {
            let inner = self.inner.lock();
            if inner.event.state != State::Uploading {
                return false;
            }
            inner.active_request_cancellation.clone()
        };
        let Some(cancellation) = cancellation else {
            return false;
        };
        cancellation.cancel();
        true
    }

    fn handle_recorder_error(&self, recorder: &Arc<Recorder>, session: u64, error: RecorderError) {
        if self.is_stopped() {
            return;
        }
        let should_report = {
            let mut inner = self.inner.lock();
            if !Arc::ptr_eq(&inner.recorder, recorder)
                || inner.active_session != session
                || !matches!(
                    inner.event.state,
                    State::Recording | State::Paused | State::Error
                )
            {
                false
            } else {
                inner.active_session = 0;
                true
            }
        };
        if should_report {
            self.set_state(State::Error, "Recording failed", Some(&error));
        }
    }

    fn toggle_pause_locked(&self) {
        let (recorder, debug) = {
            let inner = self.inner.lock();
            (inner.recorder.clone(), inner.config.hotkey_debug)
        };
        match recorder.toggle_pause() {
            Ok(RecorderState::Paused) => {
                self.set_state(State::Paused, "Recording paused", None::<&RuntimeError>)
            }
            Ok(RecorderState::Recording) => {
                self.set_state(State::Recording, "Recording resumed", None::<&RuntimeError>)
            }
            Ok(_) => {}
            Err(_) if debug => eprintln!("[hotkey] not recording; cannot pause/resume"),
            Err(_) => {}
        }
    }

    async fn cancel_recording_locked(&self) {
        let (state, recorder, session, debug) = {
            let inner = self.inner.lock();
            (
                inner.event.state,
                inner.recorder.clone(),
                inner.active_session,
                inner.config.hotkey_debug,
            )
        };
        if !matches!(state, State::Recording | State::Paused) {
            if debug {
                eprintln!("[hotkey] not recording; nothing to cancel");
            }
            return;
        }
        match recorder.cancel().await {
            Ok(_) => {
                self.clear_recording_session(&recorder, session);
                if !self.is_stopped() {
                    self.set_state(State::Idle, "Recording canceled", None::<&RuntimeError>);
                }
            }
            Err(error) => {
                if !matches!(
                    error,
                    RecorderError::NotRunning | RecorderError::WorkerStopped
                ) {
                    self.clear_recording_session(&recorder, session);
                }
                if !self.is_stopped() {
                    self.set_state(State::Error, "Cancel failed", Some(&error));
                }
            }
        }
    }

    async fn transcribe_recording(
        &self,
        result: RecordingResult,
        cancellation: &CancellationToken,
    ) {
        let Some(wav_path) = result.wav_path else {
            self.set_state(
                State::Error,
                "Recording failed",
                Some(&RuntimeError::MissingWav),
            );
            return;
        };
        let (config, client) = {
            let inner = self.inner.lock();
            (inner.config.clone(), inner.asr_client.clone())
        };
        let clipboard_write_delay = Duration::from_millis(config.clipboard_write_delay);
        let clipboard_restore_delay = Duration::from_millis(config.clipboard_restore_delay);
        let output_path = cache::recording_output_path(&wav_path, &config.container);
        if let Err(error) = self
            .converter
            .convert(
                cancellation,
                &config,
                &wav_path,
                &output_path,
                config.sampling_rate,
            )
            .await
        {
            let _ = std::fs::remove_file(&wav_path);
            let _ = std::fs::remove_file(&output_path);
            if !self.is_stopped() {
                if cancellation.is_cancelled() || matches!(error, ConvertError::Canceled) {
                    self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>);
                } else {
                    self.set_state(State::Error, "FFmpeg conversion failed", Some(&error));
                }
            }
            return;
        }
        if self.is_stopped() {
            cache::handle_cache(&config, Some(&wav_path), Some(&output_path), false, &[]);
            return;
        }
        if cancellation.is_cancelled() {
            cache::handle_cache(&config, Some(&wav_path), Some(&output_path), false, &[]);
            self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>);
            return;
        }

        let transcription = client.transcribe(cancellation, &output_path).await;
        match transcription {
            Ok(transcription) => {
                if self.is_stopped() {
                    cache::handle_cache(
                        &config,
                        Some(&wav_path),
                        Some(&output_path),
                        true,
                        &transcription.raw_response,
                    );
                    return;
                }
                if cancellation.is_cancelled() {
                    cache::handle_cache(&config, Some(&wav_path), Some(&output_path), false, &[]);
                    self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>);
                    return;
                }
                if transcription.text.is_empty() {
                    cache::handle_cache(
                        &config,
                        Some(&wav_path),
                        Some(&output_path),
                        true,
                        &transcription.raw_response,
                    );
                    self.set_state(State::Idle, "Empty result from ASR", None::<&RuntimeError>);
                    return;
                }
                let paste = clipboard::paste_text(
                    &transcription.text,
                    cancellation,
                    clipboard_write_delay,
                    clipboard_restore_delay,
                )
                .await;
                cache::handle_cache(
                    &config,
                    Some(&wav_path),
                    Some(&output_path),
                    true,
                    &transcription.raw_response,
                );
                match paste {
                    Ok(()) if !self.is_stopped() => {
                        self.set_state(State::Idle, "Transcription pasted", None::<&RuntimeError>)
                    }
                    Err(ClipboardError::Canceled) if !self.is_stopped() => {
                        self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>)
                    }
                    Err(error) if !self.is_stopped() => {
                        let message = if error.paste_was_sent_before_restore_failure() {
                            "Paste sent; clipboard restore failed"
                        } else {
                            "Paste failed"
                        };
                        self.set_state(State::Error, message, Some(&error));
                    }
                    _ => {}
                }
            }
            Err(AsrError::Canceled) => {
                cache::handle_cache(&config, Some(&wav_path), Some(&output_path), false, &[]);
                if !self.is_stopped() {
                    self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>);
                }
            }
            Err(error) => {
                if cancellation.is_cancelled() {
                    cache::handle_cache(&config, Some(&wav_path), Some(&output_path), false, &[]);
                    if !self.is_stopped() {
                        self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>);
                    }
                    return;
                }
                let raw = error.last_response().to_vec();
                if config.request_failed_notification
                    && error.is_retry_exhausted()
                    && let Err(paste_error) = clipboard::paste_text(
                        "[request failed]",
                        cancellation,
                        clipboard_write_delay,
                        clipboard_restore_delay,
                    )
                    .await
                    && !self.is_stopped()
                    && !cancellation.is_cancelled()
                {
                    eprintln!("[paste] failed: {paste_error}");
                }
                cache::handle_cache(&config, Some(&wav_path), Some(&output_path), false, &raw);
                if !self.is_stopped() {
                    if cancellation.is_cancelled() {
                        self.set_state(State::Idle, "Request canceled", None::<&RuntimeError>);
                    } else {
                        self.set_state(State::Error, "Upload failed", Some(&error));
                    }
                }
            }
        }
    }

    fn set_state<E: std::fmt::Display + ?Sized>(
        &self,
        state: State,
        message: &str,
        error: Option<&E>,
    ) {
        if self.is_stopped() {
            return;
        }
        let (event, handler) = {
            let mut inner = self.inner.lock();
            inner.event = Event {
                state,
                message: message.into(),
                error: error.map(ToString::to_string).unwrap_or_default(),
            };
            (inner.event.clone(), inner.event_handler.clone())
        };
        if let Some(handler) = handler
            && !self.is_stopped()
        {
            handler(event);
        }
    }
}

pub async fn run_file_mode(
    mut config: Config,
    converter: Arc<dyn AudioConverter>,
    input_path: &Path,
    output_path: Option<&Path>,
) -> Result<PathBuf, RuntimeError> {
    config.validate()?;
    let temp_dir = cache::initialize_cache_dir(&mut config);
    cache::cleanup_old_temp_items(&temp_dir);
    std::fs::metadata(input_path).map_err(|source| RuntimeError::InputFile {
        path: input_path.to_path_buf(),
        source,
    })?;
    let client = AsrClient::new(config.clone())?;
    let temporary = cache::temporary_output_path(&temp_dir, &config.container_extension());
    let cancellation = CancellationToken::new();
    if let Err(error) = converter
        .convert(
            &cancellation,
            &config,
            input_path,
            &temporary,
            config.sampling_rate,
        )
        .await
    {
        let _ = std::fs::remove_file(&temporary);
        return Err(error.into());
    }
    let transcription = match client.transcribe(&cancellation, &temporary).await {
        Ok(transcription) => transcription,
        Err(error) => {
            let raw = error.last_response().to_vec();
            cache::handle_cache(&config, None, Some(&temporary), false, &raw);
            return Err(error.into());
        }
    };
    let output = output_path.map(Path::to_path_buf).unwrap_or_else(|| {
        PathBuf::from(".").join(format!(
            "{}.txt",
            input_path
                .file_stem()
                .map(|stem| stem.to_string_lossy())
                .unwrap_or_default()
        ))
    });
    finish_file_mode_output(&config, &temporary, output, transcription)
}

fn finish_file_mode_output(
    config: &Config,
    temporary: &Path,
    output: PathBuf,
    transcription: Transcription,
) -> Result<PathBuf, RuntimeError> {
    if let Err(source) = std::fs::write(&output, transcription.text) {
        cache::handle_cache(
            config,
            None,
            Some(temporary),
            true,
            &transcription.raw_response,
        );
        return Err(RuntimeError::OutputFile {
            path: output,
            source,
        });
    }
    cache::handle_cache(
        config,
        None,
        Some(temporary),
        true,
        &transcription.raw_response,
    );
    Ok(output)
}

fn discard_recording(result: &RecordingResult) {
    if let Some(path) = &result.wav_path {
        let _ = std::fs::remove_file(path);
    }
}

#[cfg(test)]
mod tests {
    use async_trait::async_trait;
    use tokio::sync::Barrier;

    use super::*;

    struct NoopConverter;

    #[async_trait]
    impl AudioConverter for NoopConverter {
        async fn convert(
            &self,
            _cancellation: &CancellationToken,
            _config: &Config,
            _input: &Path,
            _output: &Path,
            _source_rate: i32,
        ) -> Result<(), ConvertError> {
            Ok(())
        }
    }

    struct CancelAwareConverter {
        started: Arc<Barrier>,
    }

    #[async_trait]
    impl AudioConverter for CancelAwareConverter {
        async fn convert(
            &self,
            cancellation: &CancellationToken,
            _config: &Config,
            _input: &Path,
            _output: &Path,
            _source_rate: i32,
        ) -> Result<(), ConvertError> {
            self.started.wait().await;
            cancellation.cancelled().await;
            Err(ConvertError::Canceled)
        }
    }

    #[tokio::test]
    async fn busy_actions_are_dropped_not_queued() {
        let runtime = Runtime::new(Config::default(), Arc::new(NoopConverter)).unwrap();
        let _guard = runtime.action_lock.clone().lock_owned().await;
        assert!(!runtime.try_toggle_recording());
        assert!(!runtime.try_toggle_pause());
        assert!(!runtime.try_cancel());
        assert_eq!(runtime.snapshot().state, State::Idle);
    }

    #[tokio::test]
    async fn uploading_request_cancel_bypasses_the_busy_action_lock() {
        let directory = tempfile::tempdir().unwrap();
        let wav_path = directory.path().join("RecordTemp_cancel.wav");
        std::fs::write(&wav_path, b"wav").unwrap();
        let started = Arc::new(Barrier::new(2));
        let runtime = Runtime::new(
            Config::default(),
            Arc::new(CancelAwareConverter {
                started: started.clone(),
            }),
        )
        .unwrap();
        let task_runtime = runtime.clone();
        let task = tokio::spawn(async move {
            let _guard = task_runtime.action_lock.clone().lock_owned().await;
            let cancellation = task_runtime.begin_active_request();
            task_runtime.set_state(
                State::Uploading,
                "Uploading ASR request",
                None::<&RuntimeError>,
            );
            task_runtime
                .transcribe_recording(
                    RecordingResult {
                        wav_path: Some(wav_path.clone()),
                        canceled: false,
                    },
                    &cancellation,
                )
                .await;
            task_runtime.clear_active_request();
            wav_path
        });

        started.wait().await;
        assert!(runtime.try_cancel());
        let wav_path = task.await.unwrap();
        assert_eq!(
            runtime.snapshot(),
            Event {
                state: State::Idle,
                message: "Request canceled".into(),
                error: String::new(),
            }
        );
        assert!(!wav_path.exists());
    }

    #[tokio::test]
    async fn stopped_runtime_rejects_actions_and_late_events() {
        let runtime = Runtime::new(Config::default(), Arc::new(NoopConverter)).unwrap();
        runtime.set_state(State::Uploading, "uploading", None::<&RuntimeError>);
        runtime.stop();
        assert!(!runtime.try_toggle_recording());
        runtime.set_state(State::Error, "late", Some(&RuntimeError::Stopped));
        assert_eq!(runtime.snapshot().state, State::Uploading);
    }

    #[tokio::test]
    async fn reload_is_only_allowed_when_idle_or_error() {
        let runtime = Runtime::new(Config::default(), Arc::new(NoopConverter)).unwrap();
        runtime.set_state(State::Recording, "recording", None::<&RuntimeError>);
        assert!(matches!(
            runtime.reload(Config::default()).await,
            Err(RuntimeError::CannotReload(State::Recording))
        ));
    }

    #[test]
    fn file_mode_output_failure_still_cleans_temporary_audio() {
        let directory = tempfile::tempdir().unwrap();
        let temporary = directory.path().join("RecordTemp_audio.ogg");
        std::fs::write(&temporary, b"converted").unwrap();
        let result = finish_file_mode_output(
            &Config::default(),
            &temporary,
            directory.path().to_path_buf(),
            Transcription {
                text: "hello".into(),
                raw_response: br#"{"text":"hello"}"#.to_vec(),
            },
        );
        assert!(matches!(result, Err(RuntimeError::OutputFile { .. })));
        assert!(!temporary.exists());
    }
}
