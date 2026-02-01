package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"stt/internal/app"
	"stt/internal/config"
)

func usage() {
	programName := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `用法: %s [选项]

该程序用于录音并将音频上传到 ASR 接口，识别结果可自动粘贴到当前光标。

选项:
[自定义配置文件]
  -config <string>
        指定配置文件（JSON），若未提供则默认读取 ./config.json（不存在则生成默认文件并退出）
  -file <string>
        指定音频文件，直接上传已有音频获得转录结果。
  -output <string>
        -file 模式下输出 txt 的路径（可选，默认当前目录同名 .txt）

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
  -extra-config <string>
        解析自定义请求字段并合并到向 API 端点发送的请求中，必须填写转义字符串，否则将无法解析。

[ffmpeg 转码配置]
  -codecs <string>
        音频编码器类型。默认: OPUS
  -container <string>
        音频容器类型。默认: OGG
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
        是否验证 HTTPS 证书（默认开启）

[热键配置]
  -start-key <string>
        开始/停止热键（例如 "alt+q"）
  -pause-key <string>
        暂停/恢复热键（例如 "alt+s"）
  -cancel-key <string>
        取消录音热键（例如 "esc"）
  -hotkeyhook <true|false>
        是否使用低级键盘钩子 (WH_KEYBOARD_LL) 来独占热键（默认关闭）。

[缓存配置]
  -cache-dir <string>
        设置缓存目录。启用后如不存在路径会尝试自动创建。
  -keep-cache <true|false>
        是否启用临时文件保存和转录记录回写（默认关闭）。此选项必须启用 -cache-dir 才会生效。

[系统通知配置]
  -notification <true|false>
        是否启用 Windows 通知（默认开启）
  -request-failed-notification <true|false>
        仅录音模式下：上传重试耗尽后，粘贴占位符 [request failed]（默认关闭）

[DEBUG 配置]
  -ffmpeg-debug <true|false>
        是否启用 FFmpeg 详情（默认关闭）。
  -record-debug <true|false>
        是否启用录音子系统的调试输出（默认关闭）。
  -hotkey-debug <true|false>
        是否启用热键/消息循环的调试输出（默认开启）。
  -upload-debug <true|false>
        是否启用上传过程的调试输出（默认关闭）。

  -h, -help, -?
        显示帮助信息

说明:
- 配置优先级：命令行标志 > 配置文件 > 默认值
- sampling-rate 单位为 Hz； bit-rate 单位为 kbps； sampling-rate-depth 单位为 bits
- TEXT_PATH 使用点分法并支持方括号索引（例如 data.items[0].value）
- 程序启动时会清理当前目录下所有以 RecordTemp_ 开头的临时文件

`, programName)
}

func main() {
	flag.Usage = usage
	flagConfigPath := flag.String("config", "", "path to config JSON")
	flagFilePath := flag.String("file", "", "path to existing audio file to upload")

	fv := config.BindFlags(flag.CommandLine)

	help := flag.Bool("h", false, "show help")
	help2 := flag.Bool("help", false, "show help")
	help3 := flag.Bool("?", false, "show help")

	flag.Parse()
	if *help || *help2 || *help3 {
		usage()
		return
	}

	var cfg config.Config
	if *flagConfigPath != "" {
		confFromFile, err := config.Load(*flagConfigPath)
		if err != nil {
			fmt.Printf("[main] failed to load config '%s': %v\n", *flagConfigPath, err)
			os.Exit(1)
		}
		cfg = confFromFile
	} else {
		if _, err := os.Stat("config.json"); err == nil {
			confFromFile, err := config.Load("config.json")
			if err != nil {
				fmt.Printf("[main] failed to load existing config.json: %v\n", err)
				os.Exit(1)
			}
			cfg = confFromFile
		} else if os.IsNotExist(err) {
			if !fv.AnySet() {
				if err := config.SaveDefault("config.json"); err != nil {
					fmt.Printf("[main] failed to write default config: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("[main] default config created at %s. Please edit it and re-run.\n", "config.json")
				return
			}
			cfg = config.DefaultConfig()
		} else {
			fmt.Printf("[main] failed to stat config.json: %v\n", err)
			os.Exit(1)
		}
	}

	config.ApplyFlags(&cfg, fv)

	if err := config.Validate(&cfg); err != nil {
		fmt.Printf("[main] invalid config: %v\n", err)
		os.Exit(1)
	}

	config.InitCacheDir(&cfg)

	if *flagFilePath != "" {
		if err := app.RunFileMode(cfg, *flagFilePath, fv.OutputPath); err != nil {
			fmt.Fprintf(os.Stderr, "[main] file mode failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := app.RunRecordMode(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[main] record mode failed: %v\n", err)
		os.Exit(1)
	}
}
