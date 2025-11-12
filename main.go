package main

// stt.go - Go rewrite of the provided Python stt.py
//
// Features implemented:
// - JSON config file support (-config)
// - Command-line flags override config values
// - Default config.json generation (with empty values) when no config & no flags
// - PortAudio-based recording (requires PortAudio C library & github.com/gordonklaus/portaudio binding)
// - Temporary files prefixed with RecordTemp_<uuid>
// - Startup cleanup of RecordTemp_* files in current directory
// - ffmpeg external call to convert WAV -> opus/ogg
// - Upload with exponential backoff retries
// - Clipboard backup/write/restore and simulated Ctrl+V paste
// - Windows notifications (beeep) when NOTIFICATION = true
// - Global hotkeys to start/stop, pause/resume, cancel (uses github.com/moutend/go-hook or similar - see notes)
//
// Build notes:
// - This program uses cgo via PortAudio. To build a static exe you must have PortAudio static library available
//   and set CGO_ENABLED=1. See README notes for details (provided separately).
//
// Limitations/Notes:
// - Hotkey implementation depends on a global hook library that may require additional Windows permissions.
// - PortAudio requires the native PortAudio library installed on the system.
// - ffmpeg must be on PATH.
//
// Author: Joey

import (
    "bytes"
    "crypto/tls"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "syscall"
    "time"
    "unsafe"

    "github.com/atotto/clipboard"
    "github.com/gen2brain/beeep"
    "github.com/go-audio/audio"
    "github.com/go-audio/wav"
    "github.com/google/uuid"
    "github.com/gordonklaus/portaudio"
    "github.com/micmonay/keybd_event"
    "golang.org/x/net/http2"
)

// Config holds configurable parameters.
type Config struct {
    APIEndpoint           string  `json:"API_ENDPOINT"`
    Token                 string  `json:"TOKEN"`
    Model                 string  `json:"MODEL"`
    Language              string  `json:"LANGUAGE"`
    Prompt                string  `json:"PROMPT"`
    TEXTPath              string  `json:"TEXT_PATH"`
    ExtraConfig           string  `json:"ExtraConfig"`
    Channels              int     `json:"CHANNELS"`
    SAMPLING_RATE         int     `json:"SAMPLING_RATE"`
    SAMPLING_RATE_DEPTH   int     `json:"SAMPLING_RATE_DEPTH"`
    BIT_RATE              int     `json:"BIT_RATE"`
    CODECS                string  `json:"CODECS"`
    CONTAINER             string  `json:"CONTAINER"`
    RequestTimeout        int     `json:"REQUEST_TIMEOUT"`
    MaxRetry              int     `json:"MAX_RETRY"`
    RetryBaseDelay        float64 `json:"RETRY_BASE_DELAY"`
    EnableHTTP2           bool    `json:"ENABLE_HTTP2"`
    VerifySSL             bool    `json:"VERIFY_SSL"`
    HotKeyHook            bool    `json:"HOTKEY_HOOK"`
    StartKey              string  `json:"START_KEY"`
    PauseKey              string  `json:"PAUSE_KEY"`
    CancelKey             string  `json:"CANCEL_KEY"`
    CacheDir              string  `json:"CACHE_DIR"`
    KeepCache             bool    `json:"KEEP_CACHE"`
    Notification          bool    `json:"NOTIFICATION"`
    FFMPEG_DEBUG          bool    `json:"FFMPEG_DEBUG"`
    RECORD_DEBUG          bool    `json:"RECORD_DEBUG"`
    HOTKEY_DEBUG          bool    `json:"HOTKEY_DEBUG"`
    UPLOAD_DEBUG          bool    `json:"UPLOAD_DEBUG"`
}

// Global runtime state
var (
    cfg             Config
    flagConfigPath  string
    flagFilePath    string
    flagOverrides   = map[string]*string{}
    isRecording     = false
    isPaused        = false
    cancelRequested = false
    mu              sync.Mutex

    // temp filenames for current recording
    currentWav string
    currentOgg string

    // shared HTTP transport and client for connection reuse
    httpTransport *http.Transport
    httpClient    *http.Client

    // parsed ExtraConfig JSON (root-level fields to merge into upload form)
    extraConfigMap map[string]interface{}
)

// defaultConfig returns a Config with default values (mostly empty as requested)
func defaultConfig() Config {
    return Config{
        APIEndpoint:         "",
        Token:               "",
        Model:               "",
        Language:            "",
        Prompt:              "",
        TEXTPath:            "text",
        ExtraConfig:         "",
        Channels:            1,
        SAMPLING_RATE:       16000,
        SAMPLING_RATE_DEPTH: 16,
        BIT_RATE:            128,
        CODECS:              "opus",
        CONTAINER:           "ogg",
        RequestTimeout:      30,
        MaxRetry:            3,
        RetryBaseDelay:      0.5,
        EnableHTTP2:         true,
        VerifySSL:           true,
        HotKeyHook:          false,
        StartKey:            "alt+q",
        PauseKey:            "alt+s",
        CancelKey:           "esc",
        CacheDir:            "",
        KeepCache:           false,
        Notification:        true,
        FFMPEG_DEBUG:        false,
        RECORD_DEBUG:        false,
        HOTKEY_DEBUG:        true,
        UPLOAD_DEBUG:        false,
    }
}

func usage() {
    programName := filepath.Base(os.Args[0])
    fmt.Fprintf(os.Stderr, `用法: %s [选项]

该程序用于录音并将音频上传到 ASR 接口，识别结果可自动粘贴到当前光标。

选项:
[自定义配置文件]
  -config <string>
        指定配置文件（JSON），若未提供则默认读取 ./config.json（不存在则生成默认文件并退出）
  -file <string>
        指定音频文件，搭配 -config 参数一起使用，直接上传已有音频获得转录结果。

[API 端点配置]
  -api-endpoint <string>
        ASR 接口 URL (e.g. https://api.example/v1/audio/transcriptions)
  -token <string>
        授权 Token（Bearer）
  -model <string>
        模型名称
  -language <string>
        识别语言 (e.g. zh)
  -prompt <string>
        识别提示文本（可选）
  -text-path <string>
        JSON 路径，用于从 ASR 返回的 JSON 中抽取文本（点分 + 数组下标语法）
        默认: "text"
        示例:
          "text"
          "text[0].test[0]"
          "results[0].alternatives[0].transcript"
  -extra-config <string>
        解析自定义请求字段并合并到向 API 端点发送的请求中，必须填写转义字符串，否则将无法解析。
        默认: ""
        示例:
          "{\"language_hints\":[\"zh\",\"en\",\"ja\"]}"
          一个 JSON 格式的转义后字符串，允许使用数组。
          将会在请求体 payload 中加入根字段 language_hints，其值为数组。
          若存在同名字段，-extra-config 中的字段优先级高于预设字段。

[ffmpeg 转码配置]
  -codecs <string>
        音频编码器类型。默认: OPUS
        允许值: OPUS, WAVPACK, AAC, AC3, EAC3, MP3, MP2, MP1, FLAC, ALAC, PCM, VORBIS, VORB, ADPCM, AMR, PCM_F32BE, PCM_F32LE, PCM_F64BE, PCM_F64LE, PCM_S16BE, PCM_S16LE, PCM_S24BE, PCM_S24LE, PCM_S32BE, PCM_S32LE, PCM_S64BE, PCM_S64LE, PCM_S8
  -container <string>
        音频容器类型。默认: OGG
        允许容器示例: WAV, AC3, AC4, OGG, OGA, MP3, FLAC, EAC3, AAC, M4A, MP4, OPUS, WEBM, S8, S16BE, S16LE, S24BE, S24LE, S32BE, S32LE, F32BE, F32LE, F64BE, F64LE
  -channels <int>
        音频通道数（默认 1）
  -sampling-rate <int>
        采样率（Hz，默认 16000 Hz）
  -sampling-rate-depth <int>
        采样精度（bits，默认 16；允许值：8,16,24,32）
  -bit-rate <int>
        目标音频比特率（kbps，默认 128 kbps）

[网络请求配置]
  -request-timeout <int>
        请求超时秒数（默认 30）
  -max-retry <int>
        上传最大重试次数（默认 3）
  -retry-base-delay <float>
        重试基准延迟秒（默认 0.5）
  -enable-http2 <true|false>
        是否启用 HTTP/2（默认开启）
  -verify-ssl <true|false>
        是否验证 HTTPS 证书（默认开启）。设置为 false 时会跳过 TLS 证书验证（不安全）。

[热键配置]
  -start-key <string>
        开始/停止热键（例如 "alt+q"）
  -pause-key <string>
        暂停/恢复热键（例如 "alt+s"）
  -cancel-key <string>
        取消录音热键（例如 "esc"）
  -hotkeyhook <true|false>
        是否使用低级键盘钩子 (WH_KEYBOARD_LL) 来独占热键（默认关闭）。

  支持的热键键名与写法（大小写不敏感；修饰键与主键用 '+' 连接，例如 "ctrl+numpad1"）:
    - 修饰键: ctrl, alt, shift, win （别名：control, menu, meta, super）
    - 顶排数字键（top-row）: 0 1 2 3 4 5 6 7 8 9  （示例: "ctrl+1" 表示顶排数字 1）
    - 字母键: a..z （示例: "ctrl+a"）
    - 功能键: F1..F24 （示例: "ctrl+F5"）
    - 命名键: esc/escape, enter/return, space, tab, backspace, insert, delete, home, end, pageup, pagedown, left, up, right, down
    - 小键盘数字（建议写法）: numpad0..numpad9（同义别名: num0..num9, kp0..kp9）。示例: "ctrl+numpad1" 或 "ctrl+num1"
    - 小键盘运算键（请使用别名，不要在 token 内使用字面 '+' 或 '-'）:
        * 加号（NumPad +）: add, plus, kpadd   （示例: "ctrl+add"）
        * 减号（NumPad -）: subtract, minus, kpsubtract   （示例: "alt+subtract"）
    - 语法注意:
        * '+' 字符用于分隔修饰键与主键；不要把 '+' 或 '-' 写入单个 token（例如请勿使用 "numpad+" 或 "numpad-"）。
        * NumLock 状态可能影响小键盘按键在系统层面发出的虚拟键（VK）。为了得到一致行为，建议启用 NumLock；若需在 NumLock=off 时支持，请绑定相应的导航键名（如 "home","end","left" 等）。

[缓存配置]
  -cache-dir <string>
        设置缓存目录。启用后如不存在路径会尝试自动创建。
  -keep-cache <true|false>
        是否启用临时文件保存和转录记录回写（默认关闭）。此选项必须启用 -cache-dir 才会生效。

[系统通知配置]
  -notification <true|false>
        是否启用 Windows 通知（默认开启）

[DEBUG 配置]
  -ffmpeg-debug <true|false>
        是否启用 FFmpeg 详情（默认关闭）。启用后会输出 ffmpeg 执行命令以便调试转码参数。
  -record-debug <true|false>
        是否启用录音子系统的调试输出（默认关闭）。启用后会打印 [record] 前缀的详细日志。
  -hotkey-debug <true|false>
        是否启用热键/消息循环的调试输出（默认开启）。启用后会打印热键解析与 WM_HOTKEY 消息信息。
  -upload-debug <true|false>
        是否启用上传过程的调试输出（默认关闭）。启用后会打印 [upload] 前缀的详细请求/响应信息。

  -h, -help, -?
        显示帮助信息

示例:
  %s -config config.json
  %s -api-endpoint https://api.example/v1/transcribe -token sk-xxx -notification=true
  %s -codecs OPUS -container OGG -sampling-rate 16000 -sampling-rate-depth 16 -bit-rate 128

说明:
- 配置优先级：命令行标志 > 配置文件 > 默认值
- sampling-rate 单位为 Hz； bit-rate 单位为 kbps； sampling-rate-depth 单位为 bits
- TEXT_PATH 使用点分法并支持方括号索引（例如 data.items[0].value）
- 程序启动时会清理当前目录下所有以 RecordTemp_ 开头的临时文件

`, programName, programName, programName)
}

// loadConfig loads config from JSON file if provided.
func loadConfig(path string) (Config, error) {
    cfg := defaultConfig()
    if path == "" {
        return cfg, nil
    }
    f, err := os.Open(path)
    if err != nil {
        return cfg, err
    }
    defer f.Close()
    dec := json.NewDecoder(f)
    if err := dec.Decode(&cfg); err != nil {
        return cfg, err
    }
    return cfg, nil
}

// saveDefaultConfig writes a default config.json into cwd (overwrite if exists).
func saveDefaultConfig(path string) error {
    cfg := defaultConfig()
    b, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, b, 0644)
}

// validateConfig verifies config fields and returns an error if any value is invalid.
// It enforces allowed values for sampling depth, codecs, and containers and checks numeric ranges.
func validateConfig(cfg *Config) error {
    // Channels: require at least 1, allow up to 8
    if cfg.Channels < 1 || cfg.Channels > 8 {
        return fmt.Errorf("invalid Channels: %d (allowed 1..8)", cfg.Channels)
    }

    // Sampling rate must be positive
    if cfg.SAMPLING_RATE <= 0 {
        return fmt.Errorf("invalid SAMPLING_RATE: %d (must be > 0)", cfg.SAMPLING_RATE)
    }

    // Sampling depth (bits) allowed set
    allowedDepth := map[int]bool{8: true, 16: true, 24: true, 32: true}
    if !allowedDepth[cfg.SAMPLING_RATE_DEPTH] {
        return fmt.Errorf("invalid SAMPLING_RATE_DEPTH: %d (allowed: 8,16,24,32)", cfg.SAMPLING_RATE_DEPTH)
    }

    // Bit rate must be positive
    if cfg.BIT_RATE <= 0 {
        return fmt.Errorf("invalid BIT_RATE: %d (must be > 0)", cfg.BIT_RATE)
    }

    // Allowed codecs (lowercase)
    allowedCodecs := map[string]bool{
        "opus":      true,
        "libopus":   true,
        "wavpack":   true,
        "aac":       true,
        "ac3":       true,
        "eac3":      true,
        "mp3":       true,
        "mp2":       true,
        "mp1":       true,
        "flac":      true,
        "alac":      true,
        "pcm":       true,
        "vorbis":    true,
        "libvorbis": true,
        "vorb":      true,
        "adpcm":     true,
        "amr":       true,
        "pcm_f32be": true,
        "pcm_f32le": true,
        "pcm_f64be": true,
        "pcm_f64le": true,
        "pcm_s16be": true,
        "pcm_s16le": true,
        "pcm_s24be": true,
        "pcm_s24le": true,
        "pcm_s32be": true,
        "pcm_s32le": true,
        "pcm_s64be": true,
        "pcm_s64le": true,
        "pcm_s8":    true,
    }
    if !allowedCodecs[strings.ToLower(cfg.CODECS)] {
        return fmt.Errorf("invalid CODECS: %s (allowed: OPUS, LIBOPUS, WAVPACK, AAC, AC3, EAC3, MP3, MP2, MP1, FLAC, ALAC, PCM, VORBIS, LIBVORBIS, VORB, ADPCM, AMR, PCM_F32BE, PCM_F32LE, PCM_F64BE, PCM_F64LE, PCM_S16BE, PCM_S16LE, PCM_S24BE, PCM_S24LE, PCM_S32BE, PCM_S32LE, PCM_S64BE, PCM_S64LE, PCM_S8)", cfg.CODECS)
    }

    // Allowed containers (lowercase)
    allowedContainers := map[string]bool{
        "wav":    true,
        "ac3":    true,
        "ac4":    true,
        "ogg":    true,
        "oga":    true,
        "mp3":    true,
        "flac":   true,
        "eac3":   true,
        "aac":    true,
        "m4a":    true,
        "mp4":    true,
        "opus":   true,
        "webm":   true,
        "s8":     true,
        "s16be":  true,
        "s16le":  true,
        "s24be":  true,
        "s24le":  true,
        "s32be":  true,
        "s32le":  true,
        "f32be":  true,
        "f32le":  true,
        "f64be":  true,
        "f64le":  true,
    }
    if !allowedContainers[strings.ToLower(cfg.CONTAINER)] {
        return fmt.Errorf("invalid CONTAINER: %s (allowed: WAV, AC3, AC4, OGG, OGA, MP3, FLAC, EAC3, AAC, M4A, MP4, OPUS, WEBM, S8, S16BE, S16LE, S24BE, S24LE, S32BE, S32LE, F32BE, F32LE, F64BE, F64LE)", cfg.CONTAINER)
    }

    return nil
}

// mergeFlags applies present flags (non-empty) to config (safe nil-checking and aliases)
func mergeFlags(cfg *Config) {
    // helper to read a string flag safely
    getStr := func(key string) (string, bool) {
        if p, ok := flagOverrides[key]; ok && p != nil && *p != "" {
            return *p, true
        }
        return "", false
    }

    if v, ok := getStr("api-endpoint"); ok {
        cfg.APIEndpoint = v
    }
    if v, ok := getStr("token"); ok {
        cfg.Token = v
    }
    if v, ok := getStr("model"); ok {
        cfg.Model = v
    }
    if v, ok := getStr("language"); ok {
        cfg.Language = v
    }
    if v, ok := getStr("prompt"); ok {
        cfg.Prompt = v
    }
    // codec/container
    if v, ok := getStr("codecs"); ok {
        cfg.CODECS = v
    }
    if v, ok := getStr("container"); ok {
        cfg.CONTAINER = v
    }

    // channels
    if v, ok := getStr("channels"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.Channels = n
        }
    }

    // sampling rate: prefer "sampling-rate", fallback to deprecated "rate"
    if v, ok := getStr("sampling-rate"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.SAMPLING_RATE = n
        }
    } else if v, ok := getStr("rate"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.SAMPLING_RATE = n
        }
    }

    // sampling depth
    if v, ok := getStr("sampling-rate-depth"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.SAMPLING_RATE_DEPTH = n
        }
    }

    // bit rate
    if v, ok := getStr("bit-rate"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.BIT_RATE = n
        }
    }

    // request timeout and retries
    if v, ok := getStr("request-timeout"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.RequestTimeout = n
        }
    }
    if v, ok := getStr("max-retry"); ok {
        if n, err := strconv.Atoi(v); err == nil {
            cfg.MaxRetry = n
        }
    }
    if v, ok := getStr("retry-base-delay"); ok {
        if f, err := strconv.ParseFloat(v, 64); err == nil {
            cfg.RetryBaseDelay = f
        }
    }

    // hotkeys & cache & notification
    if v, ok := getStr("start-key"); ok {
        cfg.StartKey = v
    }
    if v, ok := getStr("pause-key"); ok {
        cfg.PauseKey = v
    }
    if v, ok := getStr("cancel-key"); ok {
        cfg.CancelKey = v
    }
    if v, ok := getStr("cache-dir"); ok {
        cfg.CacheDir = v
    }
    if v, ok := getStr("notification"); ok {
        l := strings.ToLower(v)
        cfg.Notification = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("ffmpeg-debug"); ok {
        l := strings.ToLower(v)
        cfg.FFMPEG_DEBUG = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("record-debug"); ok {
        l := strings.ToLower(v)
        cfg.RECORD_DEBUG = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("hotkey-debug"); ok {
        l := strings.ToLower(v)
        cfg.HOTKEY_DEBUG = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("hotkeyhook"); ok {
        l := strings.ToLower(v)
        cfg.HotKeyHook = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("upload-debug"); ok {
        l := strings.ToLower(v)
        cfg.UPLOAD_DEBUG = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("keep-cache"); ok {
        l := strings.ToLower(v)
        cfg.KeepCache = (l == "1" || l == "true" || l == "yes")
    }
    if v, ok := getStr("extra-config"); ok {
        cfg.ExtraConfig = v
    }
}

func cleanupOldTempFiles() {
    // Use configured cache dir if present, otherwise fall back to cwd.
    dir := tempDir()
    entries, err := os.ReadDir(dir)
    if err != nil {
        fmt.Printf("[cleanup] read dir '%s' failed: %v\n", dir, err)
        return
    }
    for _, e := range entries {
        name := e.Name()
        if strings.HasPrefix(name, "RecordTemp_") {
            path := filepath.Join(dir, name)
            if err := os.Remove(path); err != nil {
                fmt.Printf("[cleanup] failed remove %s: %v\n", path, err)
            } else {
                fmt.Printf("[cleanup] removed %s\n", path)
            }
        }
    }
}

// generateTempFilenames sets currentWav/currentOgg with RecordTemp_<uuid>
func generateTempFilenames() {
    id := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
    baseWav := fmt.Sprintf("RecordTemp_%s.wav", id)
    // choose extension based on configured container (default to ogg)
    ext := containerExt(strings.ToLower(cfg.CONTAINER))
    baseOut := fmt.Sprintf("RecordTemp_%s.%s", id, ext)
    dir := tempDir()
    currentWav = filepath.Join(dir, baseWav)
    currentOgg = filepath.Join(dir, baseOut)
}

// tempDir returns the directory to use for temporary files. Prefers cfg.CacheDir
// when set and valid; otherwise falls back to the current working directory.
func tempDir() string {
    if cfg.CacheDir != "" {
        return cfg.CacheDir
    }
    cwd, _ := os.Getwd()
    return cwd
}

// initCacheDir validates/creates the configured cache directory. Behavior:
// - If cfg.CacheDir is empty: do nothing.
// - Else convert to absolute path and:
//   * if it exists and is a directory -> use it
//   * if it exists and is not a directory -> warn and fall back to cwd
//   * if it does not exist -> attempt to create it; on failure fall back to cwd
func initCacheDir() {
    if cfg.CacheDir == "" {
        return
    }
    abs, err := filepath.Abs(cfg.CacheDir)
    if err != nil {
        fmt.Printf("[main] cache-dir path invalid '%s': %v. Falling back to cwd.\n", cfg.CacheDir, err)
        cfg.CacheDir = ""
        return
    }
    info, err := os.Stat(abs)
    if err == nil {
        if !info.IsDir() {
            fmt.Printf("[main] cache-dir '%s' exists but is not a directory. Falling back to cwd.\n", abs, err)
            cfg.CacheDir = ""
            return
        }
        // exists and is a directory -> use it
        cfg.CacheDir = abs
        fmt.Printf("[main] using existing cache-dir: %s\n", cfg.CacheDir)
        return
    } else if os.IsNotExist(err) {
        // try to create
        if err := os.MkdirAll(abs, 0755); err != nil {
            fmt.Printf("[main] cannot create cache-dir '%s': %v. Falling back to cwd.\n", abs, err)
            cfg.CacheDir = ""
            return
        }
        cfg.CacheDir = abs
        fmt.Printf("[main] created and using cache-dir: %s\n", cfg.CacheDir)
        return
    } else {
        // other stat error
        fmt.Printf("[main] cannot access cache-dir '%s': %v. Falling back to cwd.\n", abs, err)
        cfg.CacheDir = ""
        return
    }
}

 // containerExt maps container names to file extensions (lowercase)
func containerExt(container string) string {
    c := strings.ToLower(container)
    switch c {
    case "wav":
        return "wav"
    case "ac3":
        return "ac3"
    case "ac4":
        return "ac4"
    case "ogg":
        return "ogg"
    case "oga":
        return "oga"
    case "mp3":
        return "mp3"
    case "flac":
        return "flac"
    case "eac3":
        return "eac3"
    case "aac":
        return "aac"
    case "m4a":
        return "m4a"
    case "mp4":
        return "mp4"
    case "opus":
        return "opus"
    case "webm":
        return "webm"
    case "s8", "s16be", "s16le", "s24be", "s24le", "s32be", "s32le",
         "f32be", "f32le", "f64be", "f64le":
        return c
    default:
        if c == "" {
            return "ogg"
        }
        return c
    }
}

// recordRoutine uses PortAudio to record until stopped
func recordRoutine(rate, channels int) {
    // Caller is expected to set isRecording=true before launching this goroutine.
    mu.Lock()
    isPaused = false
    cancelRequested = false
    mu.Unlock()

    generateTempFilenames()
    if cfg.RECORD_DEBUG {
        fmt.Printf("[record] starting, writing to %s\n", currentWav)
    }
    if cfg.Notification {
        _ = beeep.Notify("STT", "Recording started", "")
    }

    // Initialize PortAudio and stream
    if err := portaudio.Initialize(); err != nil {
        fmt.Printf("[record] portaudio init failed: %v\n", err)
        return
    }
    defer portaudio.Terminate()

    frames := make([]int16, 0)
    // buffer for reading
    in := make([]int16, 1024)

    stream, err := portaudio.OpenDefaultStream(channels, 0, float64(rate), len(in), in)
    if err != nil {
        fmt.Printf("[record] open stream failed: %v\n", err)
        return
    }
    // ensure stream is closed/stopped on all exit paths
    defer func() {
        _ = stream.Stop()
        _ = stream.Close()
    }()
    if err := stream.Start(); err != nil {
        fmt.Printf("[record] start stream failed: %v\n", err)
        return
    }

Loop:
    for {
        mu.Lock()
        if !isRecording {
            mu.Unlock()
            break
        }
        if isPaused {
            mu.Unlock()
            time.Sleep(100 * time.Millisecond)
            continue
        }
        mu.Unlock()

        if err := stream.Read(); err != nil {
            fmt.Printf("[record] stream read error: %v\n", err)
            continue
        }
        // append samples from in
        frames = append(frames, in...)
        // small sleep to yield
        time.Sleep(10 * time.Millisecond)

        mu.Lock()
        if cancelRequested {
            mu.Unlock()
            break Loop
        }
        mu.Unlock()
    }

    // stop stream
    _ = stream.Stop()
    _ = stream.Close()

    if cancelRequested {
        // cleanup and exit
        _ = os.Remove(currentWav)
        _ = os.Remove(currentOgg)
        currentWav = ""
        currentOgg = ""
        mu.Lock()
        isRecording = false
        isPaused = false
        cancelRequested = false
        mu.Unlock()
        if cfg.RECORD_DEBUG {
            fmt.Println("[record] recording canceled and temp files removed")
        }
        return
    }

    // write WAV using go-audio/wav
    if err := writeWav(currentWav, frames, rate, channels); err != nil {
        fmt.Printf("[record] write wav failed: %v\n", err)
        _ = os.Remove(currentWav)
        return
    }

    if cfg.RECORD_DEBUG {
        fmt.Printf("[record] saved wav %s\n", currentWav)
    }
    if cfg.Notification {
        _ = beeep.Notify("STT", "Recording finished", "")
    }

    // convert
    ok := convertAudioFile(currentWav, currentOgg, rate)
    if ok {
        // upload and capture response so we can persist if requested
        uploadOk, resBody := sendAudioWithRetry(currentOgg)

        // Decide whether to keep/rename or delete temp files.
        if cfg.KeepCache && cfg.CacheDir != "" {
            // Timestamp format: audio-YYYY-MM-DD-HH.MM.SS
            timestamp := time.Now().Format("2006-01-02-15.04.05")
            base := fmt.Sprintf("audio-%s", timestamp)

            // rename WAV if exists
            if currentWav != "" {
                wavExt := filepath.Ext(currentWav)
                newWav := filepath.Join(cfg.CacheDir, base+wavExt)
                if err := os.Rename(currentWav, newWav); err != nil {
                    fmt.Printf("[cache] failed to rename wav to %s: %v\n", newWav, err)
                    // attempt to remove original to avoid leaking temp files
                    _ = os.Remove(currentWav)
                } else {
                    currentWav = newWav
                }
            }

            // rename converted output if exists
            if currentOgg != "" {
                outExt := filepath.Ext(currentOgg)
                newOut := filepath.Join(cfg.CacheDir, base+outExt)
                if err := os.Rename(currentOgg, newOut); err != nil {
                    fmt.Printf("[cache]failed to rename output to %s: %v\n", newOut, err)
                    _ = os.Remove(currentOgg)
                } else {
                    currentOgg = newOut
                }
            }

            // persist full JSON response if upload succeeded and response body present
            if uploadOk && resBody != nil && len(resBody) > 0 {
                jsonPath := filepath.Join(cfg.CacheDir, base+".json")
                if err := os.WriteFile(jsonPath, resBody, 0644); err != nil {
                    fmt.Printf("[cache] failed to write json to %s: %v\n", jsonPath, err)
                }
            }
        } else {
            // Not keeping cache or cache dir invalid -> remove temp files
            if currentWav != "" {
                _ = os.Remove(currentWav)
                currentWav = ""
            }
            if currentOgg != "" {
                _ = os.Remove(currentOgg)
                currentOgg = ""
            }
        }
    } else {
        _ = os.Remove(currentWav)
        _ = os.Remove(currentOgg)
    }

    mu.Lock()
    isRecording = false
    isPaused = false
    cancelRequested = false
    mu.Unlock()
}

// writeWav writes []int16 samples to a WAV file (mono or interleaved)
func writeWav(path string, samples []int16, rate, channels int) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()

    enc := wav.NewEncoder(f, rate, 16, channels, 1)
    buf := &audio.IntBuffer{
        Format: &audio.Format{
            NumChannels: channels,
            SampleRate:  rate,
        },
        Data:           make([]int, len(samples)),
        SourceBitDepth: 16,
    }
    for i := range samples {
        buf.Data[i] = int(samples[i])
    }
    if err := enc.Write(buf); err != nil {
        enc.Close()
        return err
    }
    return enc.Close()
}

// convertAudioFile -- converts WAV to selected codec/container using cfg settings.
// It uses global cfg to decide codec, bitrate, sample rate, channels and sample depth.
func convertAudioFile(inWav, outPath string, rate int) bool {
    // Build ffmpeg args based on configured codec/container and audio params
    codecKey := strings.ToLower(cfg.CODECS)
    channels := cfg.Channels
    if channels <= 0 {
        channels = 1
    }
    sr := cfg.SAMPLING_RATE
    if sr <= 0 {
        sr = rate
    }
    bitrate := cfg.BIT_RATE
    if bitrate <= 0 {
        bitrate = 128
    }
    depth := cfg.SAMPLING_RATE_DEPTH
    if depth == 0 {
        depth = 16
    }

    ffCodec, codecHasBitrate := ffmpegCodecFor(codecKey)
    if ffCodec == "" {
        fmt.Printf("[ffmpeg] unsupported codec: %s\n", cfg.CODECS)
        return false
    }

    // assemble args
    args := []string{"-y", "-i", inWav, "-ac", strconv.Itoa(channels), "-ar", strconv.Itoa(sr)}
    // sample format or pcm selection
    if strings.HasPrefix(ffCodec, "pcm_") {
        // PCM encoders are specified as codec; do not add sample_fmt usually
        args = append(args, "-c:a", ffCodec)
    } else {
        args = append(args, "-c:a", ffCodec)
        // apply bitrate if codec supports it
        if codecHasBitrate {
            args = append(args, "-b:a", fmt.Sprintf("%dk", bitrate))
        }
        // sample_fmt for non-pcm codecs (best-effort)
        switch depth {
        case 8:
            args = append(args, "-sample_fmt", "u8")
        case 16:
            args = append(args, "-sample_fmt", "s16")
        case 24:
            args = append(args, "-sample_fmt", "s24")
        case 32:
            args = append(args, "-sample_fmt", "s32")
        }
    }

    // set output format by container -- output path is used as-provided (do not append extensions)
    out := outPath
    args = append(args, out)

    if cfg.FFMPEG_DEBUG {
        fmt.Printf("[ffmpeg] executing: ffmpeg %s\n", strings.Join(args, " "))
    }
    cmd := exec.Command("ffmpeg", args...)
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        fmt.Printf("[ffmpeg] failed: %v\n%s\n", err, stderr.String())
        return false
    }
    return true
}

 // ffmpegCodecFor maps normalized codec keys to ffmpeg codec names and whether they accept bitrate flag.
func ffmpegCodecFor(key string) (string, bool) {
    k := strings.ToLower(key)
    switch k {
    case "opus", "libopus":
        return "libopus", true
    case "wavpack":
        return "wavpack", false
    case "aac":
        return "aac", true
    case "ac3":
        return "ac3", true
    case "eac3":
        return "eac3", true
    case "mp3":
        return "libmp3lame", true
    case "mp2":
        return "mp2", true
    case "mp1":
        return "mp1", true
    case "flac":
        return "flac", false
    case "alac":
        return "alac", false
    case "pcm":
        // default pcm -> 16-bit little-endian
        return "pcm_s16le", false
    case "vorbis", "libvorbis", "vorb":
        return "libvorbis", true
    case "adpcm":
        return "adpcm_ms", false
    case "amr":
        return "libopencore_amrnb", true
    case "pcm_f32be", "pcm_f32le", "pcm_f64be", "pcm_f64le",
         "pcm_s16be", "pcm_s16le", "pcm_s24be", "pcm_s24le",
         "pcm_s32be", "pcm_s32le", "pcm_s64be", "pcm_s64le",
         "pcm_s8":
        // exact pcm variants map to themselves and do not accept -b:a
        return k, false
    default:
        return "", false
    }
}

// sendAudioWithRetry uploads with exponential backoff.
// Modified to return success flag and the raw response bytes so callers can
// decide whether to keep/rename temp files and persist the JSON response.
func sendAudioWithRetry(filePath string) (bool, []byte) {
    // Fail-fast if API endpoint is not configured.
    if strings.TrimSpace(cfg.APIEndpoint) == "" {
        fmt.Println("[upload] API endpoint is empty; aborting upload")
        if cfg.Notification {
            _ = beeep.Notify("STT", "Upload failed: API endpoint empty", "")
        }
        return false, nil
    }
    try := 0
    delay := cfg.RetryBaseDelay
    var lastResp []byte
    for {
        try++
        ok, res := doUpload(filePath)
        lastResp = res
        if ok {
            // process res (assume JSON)
            text := extractTextFromResponse(res)
            if text == "" {
                fmt.Println("[upload] empty result")
                if cfg.Notification {
                    _ = beeep.Notify("STT", "Empty result from ASR", "")
                }
            } else {
                fmt.Println("[upload] success, pasting text")
                if err := pasteText(text); err != nil {
                    fmt.Printf("[paste] failed: %v\n", err)
                    if cfg.Notification {
                        _ = beeep.Notify("STT", "Paste failed", "")
                    }
                } else {
                    if cfg.Notification {
                        _ = beeep.Notify("STT", "Paste success", "")
                    }
                }
            }
            return true, res
        } else {
            if cfg.UPLOAD_DEBUG {
                fmt.Printf("[upload] attempt %d failed: %v\n", try, res)
            }
            if try >= cfg.MaxRetry {
                fmt.Printf("[upload] exceeded max retries (%d)\n", cfg.MaxRetry)
                if cfg.Notification {
                    _ = beeep.Notify("STT", "Upload failed", "")
                }
                return false, lastResp
            }
            time.Sleep(time.Duration(delay * float64(time.Second)))
            delay *= 2
        }
    }
}

func doUpload(filePath string) (bool, []byte) {
    if cfg.UPLOAD_DEBUG {
        fmt.Printf("[upload] uploading %s -> %s\n", filePath, cfg.APIEndpoint)
    }
    f, err := os.Open(filePath)
    if err != nil {
        return false, []byte(fmt.Sprintf("open file error: %v", err))
    }
    defer f.Close()

    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)
    part, err := writer.CreateFormFile("file", filepath.Base(filePath))
    if err != nil {
        return false, []byte(fmt.Sprintf("create form file error: %v", err))
    }
    if _, err := io.Copy(part, f); err != nil {
        return false, []byte(fmt.Sprintf("copy file error: %v", err))
    }
    // add fields - build base map from cfg then overlay extraConfigMap (ExtraConfig has priority)
    base := make(map[string]interface{})
    if cfg.Model != "" {
        base["model"] = cfg.Model
    }
    if cfg.Language != "" {
        base["language"] = cfg.Language
    }
    if cfg.Prompt != "" {
        base["prompt"] = cfg.Prompt
    }
    // overlay extraConfigMap (ExtraConfig has priority)
    if extraConfigMap != nil {
        for k, v := range extraConfigMap {
            base[k] = v
        }
    }
    // write fields to multipart form
    for k, v := range base {
        switch val := v.(type) {
        case string:
            _ = writer.WriteField(k, val)
        case bool:
            _ = writer.WriteField(k, fmt.Sprintf("%v", val))
        case float64:
            _ = writer.WriteField(k, fmt.Sprintf("%v", val))
        case int:
            _ = writer.WriteField(k, fmt.Sprintf("%v", val))
        default:
            // marshal complex types (arrays/objects) to JSON string
            if b, err := json.Marshal(val); err == nil {
                _ = writer.WriteField(k, string(b))
            } else {
                _ = writer.WriteField(k, fmt.Sprintf("%v", val))
            }
        }
    }
    _ = writer.Close()

    // Use shared httpClient (initialized at startup) for connection reuse.
    // Fall back to a temporary client if httpClient is nil.
    c := httpClient
    if c == nil {
        c = &http.Client{
            Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
        }
    }

    req, err := http.NewRequest("POST", cfg.APIEndpoint, body)
    if err != nil {
        return false, []byte(fmt.Sprintf("new request error: %v", err))
    }
    req.Header.Set("Content-Type", writer.FormDataContentType())
    if cfg.Token != "" {
        req.Header.Set("Authorization", "Bearer "+cfg.Token)
    }
    // add a small User-Agent
    req.Header.Set("User-Agent", "stt-go-client/1.0")

    // measure request duration to observe connection reuse benefits
    start := time.Now()
    resp, err := c.Do(req)
    elapsed := time.Since(start)
    if cfg.UPLOAD_DEBUG {
        fmt.Printf("[upload] request duration: %v\n", elapsed)
    }

    if err != nil {
        return false, []byte(fmt.Sprintf("request error: %v", err))
    }
    defer resp.Body.Close()
    respBody, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        return false, respBody
    }
    return true, respBody
}

func extractTextFromResponse(body []byte) string {
    // Parse JSON into an interface{} root
    var root interface{}
    if err := json.Unmarshal(body, &root); err != nil {
        fmt.Printf("[upload] json parse error: %v\n", err)
        return ""
    }

    // 1) Try configured TEXT_PATH if present
    // If TEXT_PATH is configured and the path exists in the response, treat that result
    // as authoritative (even if it's an empty string). Returning an explicit empty string
    // prevents falling back to other top-level fields like "success" which can be confusing.
    if cfg.TEXTPath != "" {
        if v, ok := extractByPath(root, cfg.TEXTPath); ok {
            // Return the value found at TEXT_PATH (may be empty).
            return v
        }
    }

    // 2) Fallback: look for top-level "text" key (backward compatible)
    // If the key exists, return its value (may be empty). This avoids falling back
    // to unrelated fields (e.g. "success") which can cause confusing pasted text.
    if m, ok := root.(map[string]interface{}); ok {
        // If "text" key exists at all, return its value (may be empty).
        if v, exists := m["text"]; exists {
            switch s := v.(type) {
            case string:
                return s
            case float64:
                // JSON numbers are float64
                if s == float64(int64(s)) {
                    return fmt.Sprintf("%d", int64(s))
                }
                return fmt.Sprintf("%v", s)
            case bool:
                return fmt.Sprintf("%v", s)
            default:
                // not a primitive we can convert sensibly -> fallthrough to continue checking
            }
        }
        // fallback: any non-empty string value at top level
        for _, val := range m {
            if s, ok := val.(string); ok && s != "" {
                return s
            }
        }
    }

    return ""
}

// extractByPath extracts a string value from a JSON-parsed structure using a simple
// dot-separated path that supports array indexes with square brackets, e.g. "data.text[0]".
func extractByPath(root interface{}, path string) (string, bool) {
    if path == "" {
        return "", false
    }
    parts := strings.Split(path, ".")
    cur := root
    for _, part := range parts {
        // parse part like key or key[idx][idx2] or just [idx]
        key, idxs, err := parseKeyAndIndexes(part)
        if err != nil {
            return "", false
        }

        // if key is present, descend into map
        if key != "" {
            m, ok := cur.(map[string]interface{})
            if !ok {
                return "", false
            }
            next, exists := m[key]
            if !exists {
                return "", false
            }
            cur = next
        }

        // apply indexes if any
        for _, idx := range idxs {
            arr, ok := cur.([]interface{})
            if !ok {
                return "", false
            }
            if idx < 0 || idx >= len(arr) {
                return "", false
            }
            cur = arr[idx]
        }
    }

    // At the end, attempt to convert cur to string
    switch v := cur.(type) {
    case string:
        return v, true
    case float64:
        // JSON numbers are float64; format without trailing .0 when integer
        if v == float64(int64(v)) {
            return fmt.Sprintf("%d", int64(v)), true
        }
        return fmt.Sprintf("%v", v), true
    case bool:
        return fmt.Sprintf("%v", v), true
    default:
        return "", false
    }
}

// parseKeyAndIndexes parses a token like "foo[0][1]" or "[0]" or "bar" into
// base key and slice of indexes.
func parseKeyAndIndexes(token string) (string, []int, error) {
    if token == "" {
        return "", nil, fmt.Errorf("empty token")
    }
    idxs := []int{}
    // find first '['
    br := strings.Index(token, "[")
    var key string
    if br == -1 {
        key = token
        return key, idxs, nil
    }
    key = token[:br]
    rest := token[br:]
    // parse all [n] occurrences
    for len(rest) > 0 {
        if !strings.HasPrefix(rest, "[") {
            return "", nil, fmt.Errorf("invalid index syntax in %s", token)
        }
        closePos := strings.Index(rest, "]")
        if closePos == -1 {
            return "", nil, fmt.Errorf("missing closing ] in %s", token)
        }
        numStr := rest[1:closePos]
        if numStr == "" {
            return "", nil, fmt.Errorf("empty index in %s", token)
        }
        n, err := strconv.Atoi(numStr)
        if err != nil {
            return "", nil, fmt.Errorf("invalid index '%s' in %s", numStr, token)
        }
        idxs = append(idxs, n)
        rest = rest[closePos+1:]
    }
    return key, idxs, nil
}

// pasteText writes text to clipboard, sends Ctrl+V, and restores clipboard
func pasteText(text string) error {
    orig, _ := clipboard.ReadAll()
    _ = clipboard.WriteAll(text)
    // small sleep to allow clipboard to be ready
    time.Sleep(80 * time.Millisecond)

    // simulate Ctrl+V using keybd_event
    kb, err := keybd_event.NewKeyBonding()
    if err != nil {
        return err
    }
    // Press Ctrl down
    kb.HasCTRL(true)
    kb.SetKeys(keybd_event.VK_V)
    if err := kb.Launching(); err != nil {
        return err
    }
    // restore clipboard after slight delay
    time.Sleep(120 * time.Millisecond)
    _ = clipboard.WriteAll(orig)
    return nil
}

// toggle recording controls
func toggleRecording(rate, channels int) {
    mu.Lock()
    if !isRecording {
        // set state immediately to avoid race where multiple hotkey events
        // spawn multiple goroutines before isRecording is observed true.
        isRecording = true
        // start recorder goroutine
        go recordRoutine(rate, channels)
        if cfg.HOTKEY_DEBUG {
            fmt.Println("[hotkey] recording started")
        }
    } else {
        // stop (by flipping flag)
        isRecording = false
        if cfg.HOTKEY_DEBUG {
            fmt.Println("[hotkey] recording stopped")
        }
    }
    mu.Unlock()
}

func togglePause() {
    mu.Lock()
    if !isRecording {
        if cfg.HOTKEY_DEBUG {
            fmt.Println("[hotkey] not recording; cannot pause/resume")
        }
        mu.Unlock()
        return
    }
    isPaused = !isPaused
    if cfg.HOTKEY_DEBUG {
        fmt.Println("[hotkey] paused:", isPaused)
    }
    mu.Unlock()
}

func cancelRecording() {
    mu.Lock()
    if !isRecording {
        if cfg.HOTKEY_DEBUG {
            fmt.Println("[hotkey] not recording; nothing to cancel")
        }
        mu.Unlock()
        return
    }
    cancelRequested = true
    isRecording = false
    isPaused = false
    mu.Unlock()
    // remove temp files
    if currentWav != "" {
        _ = os.Remove(currentWav)
    }
    if currentOgg != "" {
        _ = os.Remove(currentOgg)
    }
    if cfg.HOTKEY_DEBUG {
        fmt.Println("[hotkey] cancel requested and temp files removed")
    }
}

// handleHotkey executes the action associated with a hotkey id.
func handleHotkey(id int) {
    switch id {
    case 1:
        toggleRecording(cfg.SAMPLING_RATE, cfg.Channels)
    case 2:
        togglePause()
    case 3:
        cancelRecording()
    }
}

// registerHotkeys registers global hotkeys using Windows RegisterHotKey API.
// On failure the function will return an error (caller should exit).
// This implementation spawns a dedicated OS thread (LockOSThread) to register hotkeys
// and run the GetMessage loop on the same thread which is required for RegisterHotKey to work reliably.
func registerHotkeys(startKey, pauseKey, cancelKey string, rate, channels int) error {
    type hotkeyDef struct {
        id   int
        spec string
        mod  uint32
        vk   uint32
    }
    defs := []hotkeyDef{
        {id: 1, spec: startKey},
        {id: 2, spec: pauseKey},
        {id: 3, spec: cancelKey},
    }

    // channel to receive registration result from the OS-thread goroutine
    errCh := make(chan error, 1)

    go func() {
        // Ensure this goroutine runs on a single OS thread for the lifetime of the message loop
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()

        // parse and populate mods/vk in this thread
        for i := range defs {
            mod, vk, err := parseHotkey(defs[i].spec)
            if err != nil {
                errCh <- fmt.Errorf("invalid hotkey '%s': %v", defs[i].spec, err)
                return
            }
            defs[i].mod = mod
            defs[i].vk = vk
            if cfg.HOTKEY_DEBUG {
                fmt.Printf("[hotkey-debug] parsed '%s' -> mod=0x%X vk=0x%X\n", defs[i].spec, defs[i].mod, defs[i].vk)
            }
        }

        user32 := syscall.NewLazyDLL("user32.dll")
        procRegisterHotKey := user32.NewProc("RegisterHotKey")
        procUnregisterHotKey := user32.NewProc("UnregisterHotKey")
        procGetMessageW := user32.NewProc("GetMessageW")

        // Register hotkeys on this OS thread
        for _, d := range defs {
            r, _, _ := procRegisterHotKey.Call(
                0,
                uintptr(d.id),
                uintptr(d.mod),
                uintptr(d.vk),
            )
            if r == 0 {
                // unregister any previously registered
                for _, od := range defs {
                    if od.id == d.id {
                        break
                    }
                    procUnregisterHotKey.Call(0, uintptr(od.id))
                }
                errCh <- fmt.Errorf("RegisterHotKey failed for '%s' (id=%d)", d.spec, d.id)
                return
            }
            if cfg.HOTKEY_DEBUG {
                fmt.Printf("[hotkey-debug] RegisterHotKey succeeded for id=%d spec=%s\n", d.id, d.spec)
            }
        }

        // success
        if cfg.HOTKEY_DEBUG {
            fmt.Printf("[hotkey] Registered global hotkeys: start=%s pause=%s cancel=%s\n", startKey, pauseKey, cancelKey)
        }
        errCh <- nil

        // Start message loop to listen for WM_HOTKEY (0x0312) on this same OS thread
        var msg struct {
            Hwnd    uintptr
            Message uint32
            WParam  uintptr
            LParam  uintptr
            Time    uint32
            Pt_x    int32
            Pt_y    int32
        }
        const WM_HOTKEY = 0x0312
        for {
            ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
            if int32(ret) == -1 {
                // error
                fmt.Println("[hotkey] GetMessageW error; exiting hotkey loop")
                return
            }
            // debug print incoming message
            if cfg.HOTKEY_DEBUG {
                fmt.Printf("[hotkey-debug] msg: Message=0x%X WParam=0x%X LParam=0x%X\n", msg.Message, msg.WParam, msg.LParam)
            }
            if msg.Message == WM_HOTKEY {
                id := int(msg.WParam)
                if cfg.HOTKEY_DEBUG {
                    fmt.Printf("[hotkey-debug] WM_HOTKEY received id=%d\n", id)
                }
                // reuse handler
                handleHotkey(id)
            }
        }
    }()

    // wait for registration result or timeout
    select {
    case err := <-errCh:
        return err
    case <-time.After(2 * time.Second):
        return fmt.Errorf("timeout registering hotkeys")
    }
}

// startLowLevelHook installs a WH_KEYBOARD_LL hook on a dedicated OS thread.
// It parses the provided hotkey specs, builds an internal lookup keyed by vk,
// and in the hook callback intercepts matching combinations and prevents them
// from being passed on (returns non-zero). Injected events (LLKHF_INJECTED)
// are ignored so program-synthesized input is not blocked.
func startLowLevelHook(startKey, pauseKey, cancelKey string, rate, channels int) error {
    type candidate struct {
        id  int
        mod uint32
    }

    errCh := make(chan error, 1)
    go func() {
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()

        // parse hotkeys
        specs := []struct {
            id   int
            spec string
        }{
            {id: 1, spec: startKey},
            {id: 2, spec: pauseKey},
            {id: 3, spec: cancelKey},
        }

        lookup := make(map[uint32][]candidate)
        for _, s := range specs {
            mod, vk, err := parseHotkey(s.spec)
            if err != nil {
                errCh <- fmt.Errorf("invalid hotkey '%s': %v", s.spec, err)
                return
            }
            lookup[vk] = append(lookup[vk], candidate{id: s.id, mod: mod})
            if cfg.HOTKEY_DEBUG {
                fmt.Printf("[hotkey-debug] parsed '%s' -> mod=0x%X vk=0x%X\n", s.spec, mod, vk)
            }
        }

        user32 := syscall.NewLazyDLL("user32.dll")
        procSetWindowsHookExW := user32.NewProc("SetWindowsHookExW")
        procUnhookWindowsHookEx := user32.NewProc("UnhookWindowsHookEx")
        procCallNextHookEx := user32.NewProc("CallNextHookEx")
        procGetMessageW := user32.NewProc("GetMessageW")
        procGetAsyncKeyState := user32.NewProc("GetAsyncKeyState")

        const (
            WH_KEYBOARD_LL = 13
            WM_KEYDOWN     = 0x0100
            WM_KEYUP       = 0x0101
            WM_SYSKEYDOWN  = 0x0104
            WM_SYSKEYUP    = 0x0105
            LLKHF_INJECTED = 0x10
            VK_SHIFT       = 0x10
            VK_CONTROL     = 0x11
            VK_MENU        = 0x12
            VK_LWIN        = 0x5B
            VK_RWIN        = 0x5C
        )

        type KBDLLHOOKSTRUCT struct {
            vkCode      uint32
            scanCode    uint32
            flags       uint32
            time        uint32
            dwExtraInfo uintptr
        }

        // helper to check if required mod mask is currently pressed
        modsSatisfied := func(required uint32) bool {
            // if no modifiers required, consider satisfied
            if required == 0 {
                return true
            }
            // check each modifier bit
            // MOD_ALT = 0x0001, MOD_CONTROL = 0x0002, MOD_SHIFT = 0x0004, MOD_WIN = 0x0008
            // check Control
            if (required & 0x0002) != 0 {
                st, _, _ := procGetAsyncKeyState.Call(uintptr(VK_CONTROL))
                if (st & 0x8000) == 0 {
                    return false
                }
            }
            // check Alt (MENU)
            if (required & 0x0001) != 0 {
                st, _, _ := procGetAsyncKeyState.Call(uintptr(VK_MENU))
                if (st & 0x8000) == 0 {
                    return false
                }
            }
            // check Shift
            if (required & 0x0004) != 0 {
                st, _, _ := procGetAsyncKeyState.Call(uintptr(VK_SHIFT))
                if (st & 0x8000) == 0 {
                    return false
                }
            }
            // check Win (either LWIN or RWIN)
            if (required & 0x0008) != 0 {
                stL, _, _ := procGetAsyncKeyState.Call(uintptr(VK_LWIN))
                stR, _, _ := procGetAsyncKeyState.Call(uintptr(VK_RWIN))
                if (stL & 0x8000) == 0 && (stR & 0x8000) == 0 {
                    return false
                }
            }
            return true
        }

        // swallowed map to remember which vk we blocked on keydown so we can also block keyup
        swallowed := make(map[uint32]bool)

        // callback executed on the locked OS thread
        callback := syscall.NewCallback(func(nCode, wParam, lParam uintptr) uintptr {
            // per MSDN, if nCode < 0, pass to CallNextHookEx
            if int32(nCode) < 0 {
                ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
                return ret
            }

            msg := uint32(wParam)
            k := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
            vk := k.vkCode
            flags := k.flags

            // ignore injected events so program-synthesized input isn't blocked
            if (flags & LLKHF_INJECTED) != 0 {
                ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
                return ret
            }

            // KEYDOWN / SYSKEYDOWN: check for match and swallow if matched
            if msg == WM_KEYDOWN || msg == WM_SYSKEYDOWN {
                if cands, ok := lookup[vk]; ok {
                    for _, c := range cands {
                        if modsSatisfied(c.mod) {
                            // mark swallowed so we also swallow the KEYUP
                            swallowed[vk] = true
                            if cfg.HOTKEY_DEBUG {
                                fmt.Printf("[hotkey-debug] swallowed keydown vk=0x%X id=%d\n", vk, c.id)
                            }
                            // call handler asynchronously to avoid blocking hook
                            go handleHotkey(c.id)
                            // return non-zero to block further processing (exclusive)
                            return uintptr(1)
                        }
                    }
                }
            }

            // KEYUP / SYSKEYUP: if we swallowed the matching keydown, swallow the keyup as well
            if msg == WM_KEYUP || msg == WM_SYSKEYUP {
                if swallowed[vk] {
                    if cfg.HOTKEY_DEBUG {
                        fmt.Printf("[hotkey-debug] swallowed keyup vk=0x%X\n", vk)
                    }
                    delete(swallowed, vk)
                    return uintptr(1)
                }
            }

            // otherwise pass along
            ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
            return ret
        })

        // install hook
        hook, _, _ := procSetWindowsHookExW.Call(
            uintptr(WH_KEYBOARD_LL),
            callback,
            0,
            0,
        )
        if hook == 0 {
            errCh <- fmt.Errorf("SetWindowsHookExW failed")
            return
        }

        if cfg.HOTKEY_DEBUG {
            fmt.Printf("[hotkey] low-level hook installed (WH_KEYBOARD_LL)\n")
        }

        // success
        errCh <- nil

        // message loop to keep hook alive
        var msg struct {
            Hwnd    uintptr
            Message uint32
            WParam  uintptr
            LParam  uintptr
            Time    uint32
            Pt_x    int32
            Pt_y    int32
        }
        for {
            ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
            if int32(ret) == -1 {
                // error
                if cfg.HOTKEY_DEBUG {
                    fmt.Println("[hotkey] GetMessageW error; exiting low-level hook loop")
                }
                break
            }
            // GetMessageW returns 0 when WM_QUIT posted -> exit loop
            if ret == 0 {
                break
            }
            // otherwise continue looping; callback will run on this thread
        }

        // unhook before exit
        procUnhookWindowsHookEx.Call(hook)
        if cfg.HOTKEY_DEBUG {
            fmt.Println("[hotkey] low-level hook uninstalled")
        }
    }()

    // wait for installation result or timeout
    select {
    case err := <-errCh:
        return err
    case <-time.After(2 * time.Second):
        return fmt.Errorf("timeout installing low-level hook")
    }
}

// parseHotkey accepts strings like "alt+q", "ctrl+shift+F1", "esc" and returns modifier mask and virtual-key code.
const (
    // Numpad keys and common numpad operators
    VK_NUMPAD0  = 0x60
    VK_NUMPAD1  = 0x61
    VK_NUMPAD2  = 0x62
    VK_NUMPAD3  = 0x63
    VK_NUMPAD4  = 0x64
    VK_NUMPAD5  = 0x65
    VK_NUMPAD6  = 0x66
    VK_NUMPAD7  = 0x67
    VK_NUMPAD8  = 0x68
    VK_NUMPAD9  = 0x69
    VK_ADD      = 0x6B
    VK_SUBTRACT = 0x6D
)

func parseHotkey(s string) (uint32, uint32, error) {
    if s == "" {
        return 0, 0, fmt.Errorf("empty key")
    }
    parts := strings.Split(s, "+")
    for i := range parts {
        parts[i] = strings.TrimSpace(strings.ToLower(parts[i]))
    }
    var mod uint32 = 0
    var keyToken string
    if len(parts) == 1 {
        keyToken = parts[0]
    } else {
        keyToken = parts[len(parts)-1]
        for _, p := range parts[:len(parts)-1] {
            switch p {
            case "alt", "menu":
                mod |= 0x0001 // MOD_ALT
            case "ctrl", "control":
                mod |= 0x0002 // MOD_CONTROL
            case "shift":
                mod |= 0x0004 // MOD_SHIFT
            case "win", "meta", "super":
                mod |= 0x0008 // MOD_WIN
            default:
                // allow modifier order flexibility; ignore unknown here
            }
        }
    }
    // map keyToken to VK
    // letters
    if len(keyToken) == 1 {
        ch := keyToken[0]
        if ch >= 'a' && ch <= 'z' {
            return mod, uint32(ch-'a'+'A'), nil
        }
        if ch >= '0' && ch <= '9' {
            return mod, uint32(ch), nil
        }
    }
    switch keyToken {
    case "esc", "escape":
        return mod, 0x1B, nil
    case "space":
        return mod, 0x20, nil
    case "enter", "return":
        return mod, 0x0D, nil
    }
    // function keys F1-F24
    if strings.HasPrefix(keyToken, "f") {
        nStr := strings.TrimPrefix(keyToken, "f")
        if n, err := strconv.Atoi(nStr); err == nil && n >= 1 && n <= 24 {
            return mod, 0x70 + uint32(n-1), nil
        }
    }

    // numpad aliases: support multiple common forms
    // Note: do NOT use literal '+' or '-' inside a token (e.g., "numpad+" is invalid since '+' is a separator).
    // Use "add"/"plus" and "subtract"/"minus" (or platform-specific kp aliases) instead.
    switch keyToken {
    case "numpad0", "num0", "kp0":
        return mod, VK_NUMPAD0, nil
    case "numpad1", "num1", "kp1":
        return mod, VK_NUMPAD1, nil
    case "numpad2", "num2", "kp2":
        return mod, VK_NUMPAD2, nil
    case "numpad3", "num3", "kp3":
        return mod, VK_NUMPAD3, nil
    case "numpad4", "num4", "kp4":
        return mod, VK_NUMPAD4, nil
    case "numpad5", "num5", "kp5":
        return mod, VK_NUMPAD5, nil
    case "numpad6", "num6", "kp6":
        return mod, VK_NUMPAD6, nil
    case "numpad7", "num7", "kp7":
        return mod, VK_NUMPAD7, nil
    case "numpad8", "num8", "kp8":
        return mod, VK_NUMPAD8, nil
    case "numpad9", "num9", "kp9":
        return mod, VK_NUMPAD9, nil
    case "add", "plus", "kpadd":
        return mod, VK_ADD, nil
    case "subtract", "minus", "kpsubtract":
        return mod, VK_SUBTRACT, nil
    }

    // map some named keys
    named := map[string]uint32{
        "tab":       0x09,
        "backspace": 0x08,
        "insert":    0x2D,
        "delete":    0x2E,
        "home":      0x24,
        "end":       0x23,
        "pageup":    0x21,
        "pagedown":  0x22,
        "left":      0x25,
        "up":        0x26,
        "right":     0x27,
        "down":      0x28,
    }
    if v, ok := named[keyToken]; ok {
        return mod, v, nil
    }
    // lastly try uppercase single letter again defensively
    if len(keyToken) == 1 {
        return mod, uint32(strings.ToUpper(keyToken)[0]), nil
    }
    return 0, 0, fmt.Errorf("unsupported key token: %s", s)
}

func main() {
    // flags
    flag.Usage = usage
    flag.StringVar(&flagConfigPath, "config", "", "path to config JSON")
    flag.StringVar(&flagFilePath, "file", "", "path to existing audio file to upload (skips recording; reads config from file)")
    flagOverrides["api-endpoint"] = flag.String("api-endpoint", "", "API endpoint URL")
    flagOverrides["token"] = flag.String("token", "", "Authorization token")
    flagOverrides["model"] = flag.String("model", "", "model")
    flagOverrides["language"] = flag.String("language", "", "language")
    flagOverrides["prompt"] = flag.String("prompt", "", "prompt")
    flagOverrides["extra-config"] = flag.String("extra-config", "", "extra JSON config to merge into request payload")

    // codec / container / audio params
    flagOverrides["codecs"] = flag.String("codecs", "", "audio codec (e.g. OPUS, AAC, MP3, FLAC)")
    flagOverrides["container"] = flag.String("container", "", "audio container (e.g. OGG, MP3, FLAC, M4A)")
    flagOverrides["channels"] = flag.String("channels", "", "channels (int)")
    // sampling-rate kept for backward compatibility; preferred flag is sampling-rate
    flagOverrides["rate"] = flag.String("rate", "", "deprecated: rate (Hz) — use -sampling-rate")
    flagOverrides["sampling-rate"] = flag.String("sampling-rate", "", "sampling rate (Hz)")
    flagOverrides["sampling-rate-depth"] = flag.String("sampling-rate-depth", "", "sampling depth (bits)")
    flagOverrides["bit-rate"] = flag.String("bit-rate", "", "bit rate (kbps)")

    flagOverrides["request-timeout"] = flag.String("request-timeout", "", "request timeout seconds")
    flagOverrides["max-retry"] = flag.String("max-retry", "", "max retry attempts")
    flagOverrides["retry-base-delay"] = flag.String("retry-base-delay", "", "retry base delay seconds (float)")
    flagOverrides["start-key"] = flag.String("start-key", "", "start/stop hotkey")
    flagOverrides["pause-key"] = flag.String("pause-key", "", "pause/resume hotkey")
    flagOverrides["cancel-key"] = flag.String("cancel-key", "", "cancel hotkey")
    flagOverrides["cache-dir"] = flag.String("cache-dir", "", "cache directory")
    flagOverrides["notification"] = flag.String("notification", "", "enable notifications (true/false)")
    flagOverrides["ffmpeg-debug"] = flag.String("ffmpeg-debug", "", "enable ffmpeg debug output (true/false)")
    flagOverrides["record-debug"] = flag.String("record-debug", "", "enable record debug output (true/false)")
    flagOverrides["hotkey-debug"] = flag.String("hotkey-debug", "", "enable hotkey debug output (true/false)")
    flagOverrides["hotkeyhook"] = flag.String("hotkeyhook", "", "use low-level keyboard hook (true/false)")
    flagOverrides["upload-debug"] = flag.String("upload-debug", "", "enable upload debug output (true/false)")
    flagOverrides["keep-cache"] = flag.String("keep-cache", "", "keep cache files (true/false)")

    // simple help flags
    help := flag.Bool("h", false, "show help")
    help2 := flag.Bool("help", false, "show help")
    help3 := flag.Bool("?", false, "show help")

    flag.Parse()
    if *help || *help2 || *help3 {
        usage()
        return
    }

    // load config
    // Behavior:
    // - If -config is provided, load that file (error and exit on failure).
    // - Else if ./config.json exists, load it (error and exit on failure).
    // - Else (no config file present) if no meaningful flags were provided, create default config.json and exit.
    // - Else (flags provided) use defaults overridden by flags.
    if flagConfigPath != "" {
        confFromFile, err := loadConfig(flagConfigPath)
        if err != nil {
            fmt.Printf("[main] failed to load config '%s': %v\n", flagConfigPath, err)
            os.Exit(1)
        }
        cfg = confFromFile
    } else {
        // no -config specified; check for ./config.json
        if _, err := os.Stat("config.json"); err == nil {
            // config.json exists - load it and error out on parse failure
            confFromFile, err := loadConfig("config.json")
            if err != nil {
                fmt.Printf("[main] failed to load existing config.json: %v\n", err)
                os.Exit(1)
            }
            cfg = confFromFile
        } else if os.IsNotExist(err) {
            // no config.json present -> check if any flags provided
            anyFlag := false
            for _, v := range flagOverrides {
                if *v != "" {
                    anyFlag = true
                    break
                }
            }
            if !anyFlag {
                // create default config.json and exit so user can edit it
                defaultPath := "config.json"
                if err := saveDefaultConfig(defaultPath); err != nil {
                    fmt.Printf("[main] failed to write default config: %v\n", err)
                    os.Exit(1)
                }
                fmt.Printf("[main] default config created at %s. Please edit it and re-run.\n", defaultPath)
                return
            }
            // else: use defaults and merge flags below
            cfg = defaultConfig()
        } else {
            fmt.Printf("[main] failed to stat config.json: %v\n", err)
            os.Exit(1)
        }
    }
    if flagFilePath == "" {
        mergeFlags(&cfg)
    }

    // parse ExtraConfig JSON (root-level fields to merge into upload form)
    if cfg.ExtraConfig != "" {
        extraConfigMap = make(map[string]interface{})
        if err := json.Unmarshal([]byte(cfg.ExtraConfig), &extraConfigMap); err != nil {
            fmt.Printf("[main] invalid extra-config JSON: %v\n", err)
            os.Exit(1)
        }
    }

    // optional: validate config and fail fast if invalid
    if err := validateConfig(&cfg); err != nil {
        fmt.Printf("[main] invalid config: %v\n", err)
        os.Exit(1)
    }

    // initialize cache dir if provided (validate/create; on failure fall back to cwd)
    initCacheDir()

    // cleanup old RecordTemp_*
    cleanupOldTempFiles()

    // initialize shared HTTP transport / client for connection reuse
    tr := &http.Transport{
        MaxIdleConns:          100,
        MaxIdleConnsPerHost:   100,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
    }
    if !cfg.VerifySSL {
        tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
    }
    // configure HTTP/2 if enabled
    if cfg.EnableHTTP2 {
        _ = http2.ConfigureTransport(tr)
    }
    httpTransport = tr
    httpClient = &http.Client{
        Transport: tr,
        Timeout:   time.Duration(cfg.RequestTimeout) * time.Second,
    }

    // File-mode: if a file path was provided, skip hotkeys/recording and process the file directly.
    if flagFilePath != "" {
        // validate input file
        if _, err := os.Stat(flagFilePath); err != nil {
            fmt.Printf("[main] file '%s' stat failed: %v\n", flagFilePath, err)
            if httpTransport != nil {
                httpTransport.CloseIdleConnections()
            }
            os.Exit(1)
        }

        // prepare temp filenames for conversion output
        generateTempFilenames()

        // convert input file -> currentOgg using existing convertAudioFile logic
        ok := convertAudioFile(flagFilePath, currentOgg, cfg.SAMPLING_RATE)
        if !ok {
            fmt.Printf("[main] convert failed for %s -> %s\n", flagFilePath, currentOgg)
            if httpTransport != nil {
                httpTransport.CloseIdleConnections()
            }
            os.Exit(2)
        }

        // upload converted file (sendAudioWithRetry will paste extracted text)
        uploadOk, resBody := sendAudioWithRetry(currentOgg)

        // caching/persistence similar to recordRoutine
        if cfg.KeepCache && cfg.CacheDir != "" {
            timestamp := time.Now().Format("2006-01-02-15.04.05")
            base := fmt.Sprintf("audio-%s", timestamp)

            // move/rename converted output into cache dir
            if currentOgg != "" {
                outExt := filepath.Ext(currentOgg)
                newOut := filepath.Join(cfg.CacheDir, base+outExt)
                if err := os.Rename(currentOgg, newOut); err != nil {
                    fmt.Printf("[cache] failed to move output to %s: %v\n", newOut, err)
                    _ = os.Remove(currentOgg)
                } else {
                    currentOgg = newOut
                }
            }

            // persist JSON response if upload succeeded
            if uploadOk && resBody != nil && len(resBody) > 0 {
                jsonPath := filepath.Join(cfg.CacheDir, base+".json")
                if err := os.WriteFile(jsonPath, resBody, 0644); err != nil {
                    fmt.Printf("[cache] failed to write json to %s: %v\n", jsonPath, err)
                }
            }
        } else {
            // not keeping cache -> remove temp output
            if currentOgg != "" {
                _ = os.Remove(currentOgg)
                currentOgg = ""
            }
        }

        // cleanup and exit with proper code
        if httpTransport != nil {
            httpTransport.CloseIdleConnections()
        }
        if uploadOk {
            os.Exit(0)
        } else {
            os.Exit(3)
        }
    }

    // Choose hotkey registration method based on cfg.HotKeyHook
    if cfg.HotKeyHook {
        if err := startLowLevelHook(cfg.StartKey, cfg.PauseKey, cfg.CancelKey, cfg.SAMPLING_RATE, cfg.Channels); err != nil {
            fmt.Printf("[main] failed to install low-level hook: %v\n", err)
            if httpTransport != nil {
                httpTransport.CloseIdleConnections()
            }
            os.Exit(1)
        }
    } else {
        // register global hotkeys (will error and exit if registration fails)
        if err := registerHotkeys(cfg.StartKey, cfg.PauseKey, cfg.CancelKey, cfg.SAMPLING_RATE, cfg.Channels); err != nil {
            fmt.Printf("[main] failed to register global hotkeys: %v\n", err)
            fmt.Println("[main] Please ensure the program has necessary permissions and that the hotkey configuration is valid.")
            // ensure idle connections are closed before exit
            if httpTransport != nil {
                httpTransport.CloseIdleConnections()
            }
            os.Exit(1)
        }
    }

    fmt.Println("[main] ready. Type 'start' to begin recording, 'pause' to toggle pause, 'cancel' to cancel, 'quit' to exit.")
    // main loop - wait indefinitely
    for {
        time.Sleep(time.Hour)
    }
}
