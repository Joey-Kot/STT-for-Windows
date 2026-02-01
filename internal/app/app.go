package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/http2"

	"stt/internal/asr"
	"stt/internal/audio/ffmpeg"
	"stt/internal/clipboard"
	"stt/internal/config"
	"stt/internal/hotkey"
	"stt/internal/notify"
	"stt/internal/record"
)

// RunRecordMode starts hotkeys and runs the recording loop.
func RunRecordMode(cfg config.Config) error {
	tempDir := config.TempDir(&cfg)
	cleanupOldTempFiles(tempDir)

	httpClient := newHTTPClient(cfg)
	asrClient, err := asr.New(cfg, httpClient)
	if err != nil {
		return err
	}

	recorder := record.New(cfg, tempDir)

	var actionMu sync.Mutex
	handler := func(id int) {
		actionMu.Lock()
		defer actionMu.Unlock()

		switch id {
		case 1:
			if recorder.State() == record.StateIdle {
				if err := recorder.Start(context.Background()); err != nil {
					fmt.Printf("[record] start failed: %v\n", err)
					return
				}
				if cfg.HOTKEY_DEBUG {
					fmt.Println("[hotkey] recording started")
				}
				if cfg.Notification {
					notify.Notify("STT", "Recording started")
				}
			} else {
				res, err := recorder.Stop()
				if err != nil {
					fmt.Printf("[record] stop failed: %v\n", err)
					return
				}
				if res.Canceled {
					return
				}
				if res.Err != nil {
					fmt.Printf("[record] recording error: %v\n", res.Err)
					return
				}
				if cfg.HOTKEY_DEBUG {
					fmt.Println("[hotkey] recording stopped")
				}
				if cfg.Notification {
					notify.Notify("STT", "Recording finished")
				}

				outPath := strings.TrimSuffix(res.WavPath, filepath.Ext(res.WavPath)) + "." + config.ContainerExt(cfg.CONTAINER)
				if err := ffmpeg.Convert(cfg, res.WavPath, outPath, cfg.SAMPLING_RATE); err != nil {
					fmt.Printf("[ffmpeg] failed: %v\n", err)
					_ = os.Remove(res.WavPath)
					_ = os.Remove(outPath)
					return
				}

				text, raw, err := asrClient.Transcribe(context.Background(), outPath)
				uploadOk := err == nil
				if err != nil {
					fmt.Printf("[upload] failed: %v\n", err)
					if cfg.Notification {
						notify.Notify("STT", "Upload failed")
					}
					if cfg.RequestFailedNotification {
						var re *asr.RetryExhaustedError
						if errors.As(err, &re) {
							if err := clipboard.PasteText("[request failed]"); err != nil {
								fmt.Printf("[paste] failed: %v\n", err)
							} else if cfg.Notification {
								notify.Notify("STT", "Request failed")
							}
						}
					}
				} else if text == "" {
					fmt.Println("[upload] empty result")
					if cfg.Notification {
						notify.Notify("STT", "Empty result from ASR")
					}
				} else {
					if err := clipboard.PasteText(text); err != nil {
						fmt.Printf("[paste] failed: %v\n", err)
						if cfg.Notification {
							notify.Notify("STT", "Paste failed")
						}
					} else if cfg.Notification {
						notify.Notify("STT", "Paste success")
					}
				}

				handleCache(cfg, res.WavPath, outPath, uploadOk, raw)
			}
		case 2:
			if err := recorder.TogglePause(); err != nil {
				if cfg.HOTKEY_DEBUG {
					fmt.Println("[hotkey] not recording; cannot pause/resume")
				}
				return
			}
			if cfg.HOTKEY_DEBUG {
				paused := recorder.State() == record.StatePaused
				fmt.Println("[hotkey] paused:", paused)
			}
		case 3:
			if recorder.State() == record.StateIdle {
				if cfg.HOTKEY_DEBUG {
					fmt.Println("[hotkey] not recording; nothing to cancel")
				}
				return
			}
			_, _ = recorder.Cancel()
			if cfg.HOTKEY_DEBUG {
				fmt.Println("[hotkey] cancel requested")
			}
		}
	}

	if err := hotkey.Register(cfg.StartKey, cfg.PauseKey, cfg.CancelKey, cfg.HotKeyHook, handler, cfg.HOTKEY_DEBUG); err != nil {
		return err
	}

	fmt.Println("[main] ready. Use hotkeys to start/stop/pause/cancel.")
	for {
		time.Sleep(time.Hour)
	}
}

// RunFileMode uploads an existing file and writes the result to a .txt file.
func RunFileMode(cfg config.Config, inputPath string, outputPath string) error {
	tempDir := config.TempDir(&cfg)
	cleanupOldTempFiles(tempDir)

	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("file '%s' stat failed: %w", inputPath, err)
	}

	httpClient := newHTTPClient(cfg)
	asrClient, err := asr.New(cfg, httpClient)
	if err != nil {
		return err
	}

	tempOut := tempOutputPath(tempDir, config.ContainerExt(cfg.CONTAINER))
	if err := ffmpeg.Convert(cfg, inputPath, tempOut, cfg.SAMPLING_RATE); err != nil {
		_ = os.Remove(tempOut)
		return err
	}

	text, raw, err := asrClient.Transcribe(context.Background(), tempOut)
	uploadOk := err == nil
	if err != nil {
		if cfg.Notification {
			notify.Notify("STT", "Upload failed")
		}
		handleCache(cfg, "", tempOut, uploadOk, raw)
		return err
	}

	outPath := outputPath
	if outPath == "" {
		base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
		outPath = filepath.Join(".", base+".txt")
	}

	if err := os.WriteFile(outPath, []byte(text), 0644); err != nil {
		handleCache(cfg, "", tempOut, uploadOk, raw)
		return err
	}

	handleCache(cfg, "", tempOut, uploadOk, raw)
	return nil
}

func newHTTPClient(cfg config.Config) *http.Client {
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
	if cfg.EnableHTTP2 {
		_ = http2.ConfigureTransport(tr)
	}
	return &http.Client{
		Transport: tr,
		Timeout:   time.Duration(cfg.RequestTimeout) * time.Second,
	}
}

func cleanupOldTempFiles(dir string) {
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

func handleCache(cfg config.Config, wavPath string, outPath string, uploadOk bool, resBody []byte) {
	if cfg.KeepCache && cfg.CacheDir != "" {
		timestamp := time.Now().Format("2006-01-02-15.04.05")
		base := fmt.Sprintf("audio-%s", timestamp)

		if wavPath != "" {
			wavExt := filepath.Ext(wavPath)
			newWav := filepath.Join(cfg.CacheDir, base+wavExt)
			if err := os.Rename(wavPath, newWav); err != nil {
				fmt.Printf("[cache] failed to rename wav to %s: %v\n", newWav, err)
				_ = os.Remove(wavPath)
			}
		}

		if outPath != "" {
			outExt := filepath.Ext(outPath)
			newOut := filepath.Join(cfg.CacheDir, base+outExt)
			if err := os.Rename(outPath, newOut); err != nil {
				fmt.Printf("[cache] failed to rename output to %s: %v\n", newOut, err)
				_ = os.Remove(outPath)
			}
		}

		if uploadOk && resBody != nil && len(resBody) > 0 {
			jsonPath := filepath.Join(cfg.CacheDir, base+".json")
			if err := os.WriteFile(jsonPath, resBody, 0644); err != nil {
				fmt.Printf("[cache] failed to write json to %s: %v\n", jsonPath, err)
			}
		}
	} else {
		if wavPath != "" {
			_ = os.Remove(wavPath)
		}
		if outPath != "" {
			_ = os.Remove(outPath)
		}
	}
}

func tempOutputPath(dir, ext string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	base := fmt.Sprintf("RecordTemp_%s.%s", id, ext)
	if dir == "" {
		cwd, _ := os.Getwd()
		dir = cwd
	}
	return filepath.Join(dir, base)
}
