package config

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// FlagValues holds parsed flags with explicit set tracking.
type FlagValues struct {
	APIEndpoint            string
	APIEndpointSet         bool
	Token                  string
	TokenSet               bool
	Model                  string
	ModelSet               bool
	Language               string
	LanguageSet            bool
	Prompt                 string
	PromptSet              bool
	TEXTPath               string
	TEXTPathSet            bool
	ExtraConfig            string
	ExtraConfigSet         bool
	Channels               int
	ChannelsSet            bool
	SAMPLING_RATE          int
	SAMPLING_RATESet       bool
	SAMPLING_RATE_DEPTH    int
	SAMPLING_RATE_DEPTHSet bool
	BIT_RATE               int
	BIT_RATESet            bool
	CODECS                 string
	CODECSSet              bool
	CONTAINER              string
	CONTAINERSet           bool
	RequestTimeout         int
	RequestTimeoutSet      bool
	MaxRetry               int
	MaxRetrySet            bool
	RetryBaseDelay         float64
	RetryBaseDelaySet      bool
	EnableHTTP2            bool
	EnableHTTP2Set         bool
	VerifySSL              bool
	VerifySSLSet           bool
	HotKeyHook             bool
	HotKeyHookSet          bool
	StartKey               string
	StartKeySet            bool
	PauseKey               string
	PauseKeySet            bool
	CancelKey              string
	CancelKeySet           bool
	CacheDir               string
	CacheDirSet            bool
	KeepCache              bool
	KeepCacheSet           bool
	Notification           bool
	NotificationSet        bool
	FFMPEG_DEBUG           bool
	FFMPEG_DEBUGSet        bool
	RECORD_DEBUG           bool
	RECORD_DEBUGSet        bool
	HOTKEY_DEBUG           bool
	HOTKEY_DEBUGSet        bool
	UPLOAD_DEBUG           bool
	UPLOAD_DEBUGSet        bool

	OutputPath    string
	OutputPathSet bool
}

type stringFlag struct {
	target *string
	set    *bool
}

func (s *stringFlag) String() string {
	if s == nil || s.target == nil {
		return ""
	}
	return *s.target
}

func (s *stringFlag) Set(v string) error {
	if s.target != nil {
		*s.target = v
	}
	if s.set != nil {
		*s.set = true
	}
	return nil
}

type intFlag struct {
	target *int
	set    *bool
}

func (i *intFlag) String() string {
	if i == nil || i.target == nil {
		return ""
	}
	return fmt.Sprintf("%d", *i.target)
}

func (i *intFlag) Set(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	if i.target != nil {
		*i.target = n
	}
	if i.set != nil {
		*i.set = true
	}
	return nil
}

type floatFlag struct {
	target *float64
	set    *bool
}

func (f *floatFlag) String() string {
	if f == nil || f.target == nil {
		return ""
	}
	return fmt.Sprintf("%v", *f.target)
}

func (f *floatFlag) Set(v string) error {
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return err
	}
	if f.target != nil {
		*f.target = n
	}
	if f.set != nil {
		*f.set = true
	}
	return nil
}

type boolFlag struct {
	target *bool
	set    *bool
}

func (b *boolFlag) String() string {
	if b == nil || b.target == nil {
		return ""
	}
	return fmt.Sprintf("%v", *b.target)
}

func parseBoolExt(v string) (bool, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "1", "true", "yes", "y":
		return true, nil
	case "0", "false", "no", "n":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean: %s", v)
}

func (b *boolFlag) Set(v string) error {
	n, err := parseBoolExt(v)
	if err != nil {
		return err
	}
	if b.target != nil {
		*b.target = n
	}
	if b.set != nil {
		*b.set = true
	}
	return nil
}

// BindFlags registers all flags and returns the populated FlagValues.
func BindFlags(fs *flag.FlagSet) *FlagValues {
	fv := &FlagValues{}

	fs.Var(&stringFlag{&fv.APIEndpoint, &fv.APIEndpointSet}, "api-endpoint", "API endpoint URL")
	fs.Var(&stringFlag{&fv.Token, &fv.TokenSet}, "token", "Authorization token")
	fs.Var(&stringFlag{&fv.Model, &fv.ModelSet}, "model", "model")
	fs.Var(&stringFlag{&fv.Language, &fv.LanguageSet}, "language", "language")
	fs.Var(&stringFlag{&fv.Prompt, &fv.PromptSet}, "prompt", "prompt")
	fs.Var(&stringFlag{&fv.TEXTPath, &fv.TEXTPathSet}, "text-path", "JSON path to extract text")
	fs.Var(&stringFlag{&fv.ExtraConfig, &fv.ExtraConfigSet}, "extra-config", "extra JSON config to merge into request payload")

	fs.Var(&stringFlag{&fv.CODECS, &fv.CODECSSet}, "codecs", "audio codec (e.g. OPUS, AAC, MP3, FLAC)")
	fs.Var(&stringFlag{&fv.CONTAINER, &fv.CONTAINERSet}, "container", "audio container (e.g. OGG, MP3, FLAC, M4A)")
	fs.Var(&intFlag{&fv.Channels, &fv.ChannelsSet}, "channels", "channels (int)")
	fs.Var(&intFlag{&fv.SAMPLING_RATE, &fv.SAMPLING_RATESet}, "sampling-rate", "sampling rate (Hz)")
	// deprecated alias
	fs.Var(&intFlag{&fv.SAMPLING_RATE, &fv.SAMPLING_RATESet}, "rate", "deprecated: rate (Hz) — use -sampling-rate")
	fs.Var(&intFlag{&fv.SAMPLING_RATE_DEPTH, &fv.SAMPLING_RATE_DEPTHSet}, "sampling-rate-depth", "sampling depth (bits)")
	fs.Var(&intFlag{&fv.BIT_RATE, &fv.BIT_RATESet}, "bit-rate", "bit rate (kbps)")

	fs.Var(&intFlag{&fv.RequestTimeout, &fv.RequestTimeoutSet}, "request-timeout", "request timeout seconds")
	fs.Var(&intFlag{&fv.MaxRetry, &fv.MaxRetrySet}, "max-retry", "max retry attempts")
	fs.Var(&floatFlag{&fv.RetryBaseDelay, &fv.RetryBaseDelaySet}, "retry-base-delay", "retry base delay seconds (float)")
	fs.Var(&boolFlag{&fv.EnableHTTP2, &fv.EnableHTTP2Set}, "enable-http2", "enable HTTP/2 (true/false)")
	fs.Var(&boolFlag{&fv.VerifySSL, &fv.VerifySSLSet}, "verify-ssl", "verify TLS certificates (true/false)")

	fs.Var(&stringFlag{&fv.StartKey, &fv.StartKeySet}, "start-key", "start/stop hotkey")
	fs.Var(&stringFlag{&fv.PauseKey, &fv.PauseKeySet}, "pause-key", "pause/resume hotkey")
	fs.Var(&stringFlag{&fv.CancelKey, &fv.CancelKeySet}, "cancel-key", "cancel hotkey")
	fs.Var(&boolFlag{&fv.HotKeyHook, &fv.HotKeyHookSet}, "hotkeyhook", "use low-level keyboard hook (true/false)")

	fs.Var(&stringFlag{&fv.CacheDir, &fv.CacheDirSet}, "cache-dir", "cache directory")
	fs.Var(&boolFlag{&fv.KeepCache, &fv.KeepCacheSet}, "keep-cache", "keep cache files (true/false)")

	fs.Var(&boolFlag{&fv.Notification, &fv.NotificationSet}, "notification", "enable notifications (true/false)")
	fs.Var(&boolFlag{&fv.FFMPEG_DEBUG, &fv.FFMPEG_DEBUGSet}, "ffmpeg-debug", "enable ffmpeg debug output (true/false)")
	fs.Var(&boolFlag{&fv.RECORD_DEBUG, &fv.RECORD_DEBUGSet}, "record-debug", "enable record debug output (true/false)")
	fs.Var(&boolFlag{&fv.HOTKEY_DEBUG, &fv.HOTKEY_DEBUGSet}, "hotkey-debug", "enable hotkey debug output (true/false)")
	fs.Var(&boolFlag{&fv.UPLOAD_DEBUG, &fv.UPLOAD_DEBUGSet}, "upload-debug", "enable upload debug output (true/false)")

	fs.Var(&stringFlag{&fv.OutputPath, &fv.OutputPathSet}, "output", "output txt path for -file mode")

	return fv
}

// ApplyFlags applies present flags to the config.
func ApplyFlags(cfg *Config, fv *FlagValues) {
	if fv.APIEndpointSet {
		cfg.APIEndpoint = fv.APIEndpoint
	}
	if fv.TokenSet {
		cfg.Token = fv.Token
	}
	if fv.ModelSet {
		cfg.Model = fv.Model
	}
	if fv.LanguageSet {
		cfg.Language = fv.Language
	}
	if fv.PromptSet {
		cfg.Prompt = fv.Prompt
	}
	if fv.TEXTPathSet {
		cfg.TEXTPath = fv.TEXTPath
	}
	if fv.ExtraConfigSet {
		cfg.ExtraConfig = fv.ExtraConfig
	}

	if fv.CODECSSet {
		cfg.CODECS = fv.CODECS
	}
	if fv.CONTAINERSet {
		cfg.CONTAINER = fv.CONTAINER
	}
	if fv.ChannelsSet {
		cfg.Channels = fv.Channels
	}
	if fv.SAMPLING_RATESet {
		cfg.SAMPLING_RATE = fv.SAMPLING_RATE
	}
	if fv.SAMPLING_RATE_DEPTHSet {
		cfg.SAMPLING_RATE_DEPTH = fv.SAMPLING_RATE_DEPTH
	}
	if fv.BIT_RATESet {
		cfg.BIT_RATE = fv.BIT_RATE
	}

	if fv.RequestTimeoutSet {
		cfg.RequestTimeout = fv.RequestTimeout
	}
	if fv.MaxRetrySet {
		cfg.MaxRetry = fv.MaxRetry
	}
	if fv.RetryBaseDelaySet {
		cfg.RetryBaseDelay = fv.RetryBaseDelay
	}
	if fv.EnableHTTP2Set {
		cfg.EnableHTTP2 = fv.EnableHTTP2
	}
	if fv.VerifySSLSet {
		cfg.VerifySSL = fv.VerifySSL
	}

	if fv.StartKeySet {
		cfg.StartKey = fv.StartKey
	}
	if fv.PauseKeySet {
		cfg.PauseKey = fv.PauseKey
	}
	if fv.CancelKeySet {
		cfg.CancelKey = fv.CancelKey
	}
	if fv.HotKeyHookSet {
		cfg.HotKeyHook = fv.HotKeyHook
	}

	if fv.CacheDirSet {
		cfg.CacheDir = fv.CacheDir
	}
	if fv.KeepCacheSet {
		cfg.KeepCache = fv.KeepCache
	}

	if fv.NotificationSet {
		cfg.Notification = fv.Notification
	}
	if fv.FFMPEG_DEBUGSet {
		cfg.FFMPEG_DEBUG = fv.FFMPEG_DEBUG
	}
	if fv.RECORD_DEBUGSet {
		cfg.RECORD_DEBUG = fv.RECORD_DEBUG
	}
	if fv.HOTKEY_DEBUGSet {
		cfg.HOTKEY_DEBUG = fv.HOTKEY_DEBUG
	}
	if fv.UPLOAD_DEBUGSet {
		cfg.UPLOAD_DEBUG = fv.UPLOAD_DEBUG
	}
}

// AnySet reports whether any flag was explicitly set by the user.
func (fv *FlagValues) AnySet() bool {
	return fv.APIEndpointSet ||
		fv.TokenSet ||
		fv.ModelSet ||
		fv.LanguageSet ||
		fv.PromptSet ||
		fv.TEXTPathSet ||
		fv.ExtraConfigSet ||
		fv.ChannelsSet ||
		fv.SAMPLING_RATESet ||
		fv.SAMPLING_RATE_DEPTHSet ||
		fv.BIT_RATESet ||
		fv.CODECSSet ||
		fv.CONTAINERSet ||
		fv.RequestTimeoutSet ||
		fv.MaxRetrySet ||
		fv.RetryBaseDelaySet ||
		fv.EnableHTTP2Set ||
		fv.VerifySSLSet ||
		fv.HotKeyHookSet ||
		fv.StartKeySet ||
		fv.PauseKeySet ||
		fv.CancelKeySet ||
		fv.CacheDirSet ||
		fv.KeepCacheSet ||
		fv.NotificationSet ||
		fv.FFMPEG_DEBUGSet ||
		fv.RECORD_DEBUGSet ||
		fv.HOTKEY_DEBUGSet ||
		fv.UPLOAD_DEBUGSet ||
		fv.OutputPathSet
}
