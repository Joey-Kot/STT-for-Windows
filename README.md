# STT for Windows

Windows 桌面语音转写客户端，支持 GUI 浮窗和 CLI 两种使用方式。程序通过快捷键控制录音，将音频上传到兼容 REST 接口的 ASR 服务，并把识别结果写入剪贴板后自动粘贴到当前输入位置。

## 主要特性

- GUI 桌面客户端：浮窗录音控制、minimal 工具条、系统托盘、Settings 配置界面。
- CLI 客户端：命令行参数、配置文件和热键工作流。
- 全局快捷键：开始/停止、暂停/恢复、取消录音。
- JSON 配置：GUI 可视化编辑，CLI 支持配置文件和命令行参数覆盖。
- 音频处理：PortAudio 录音，ffmpeg 转码，默认 `opus/ogg`。
- 上传与重试：支持请求超时、最大重试次数、重试延迟、HTTP/2、SSL 校验配置。
- 结果粘贴：从返回 JSON 中按 `TEXT_PATH` 抽取文本，写入剪贴板并模拟 `Ctrl+V`。
- 缓存能力：可选择保留录音、转码文件和响应 JSON。

## 下载与使用

在 GitHub Releases 的 `Latest` 中下载需要的版本：

| 文件 | 说明 |
|------|------|
| `stt-cli-windows-amd64.zip` | CLI 版本，压缩包内为 `stt.exe` |
| `stt-cli-windows-amd64.zip.sha256` | CLI 压缩包 SHA256 |
| `stt-gui-windows-amd64.zip` | GUI 版本，压缩包内为 `STT.exe` |
| `stt-gui-windows-amd64.zip.sha256` | GUI 压缩包 SHA256 |

运行前请确认：

- 系统为 Windows。
- 已准备好 ASR 接口地址、Token、模型名等必要配置。

### GUI

解压 `stt-gui-windows-amd64.zip`，运行 `STT.exe`。

GUI 版本已静态链接裁剪版 FFmpeg/libav 音频转码能力，不需要额外安装 `ffmpeg`。

GUI 启动后会显示小型浮窗：

- 麦克风按钮：开始/停止录音。
- 暂停按钮：暂停/恢复录音。
- 取消按钮：取消当前录音。
- `-` 按钮：进入 minimal 工具条。
- minimal 工具条中的 `+` 按钮：恢复完整浮窗。
- 托盘菜单：`Minimal`、`Settings`、`Quit`。

GUI 默认配置路径为：

```text
%APPDATA%\stt\config.json
```

首次启动时如果该文件不存在，GUI 会自动生成默认配置。通过托盘菜单或浮窗设置按钮打开 `Settings` 后，可以编辑并保存 ASR JSON 配置。保存配置时需要处于空闲状态，录音、暂停或上传中不允许保存。

### CLI

解压 `stt-cli-windows-amd64.zip`，在终端运行：

```powershell
.\stt.exe
```

CLI 版本会调用系统 `PATH` 中的 `ffmpeg`，运行前请确认可在终端中执行 `ffmpeg -version`。

CLI 默认查找当前目录下的 `config.json`。如果当前目录没有 `config.json` 且没有提供任何命令行参数，程序会生成默认配置文件并退出。

常见用法：

```powershell
.\stt.exe -config config.json
```

```powershell
.\stt.exe -api-endpoint https://api.example/v1/transcribe -token sk-xxx -file sample.wav
```

## 默认快捷键

| 动作 | 默认快捷键 |
|------|------------|
| 开始/停止录音 | `ctrl+alt+q` |
| 暂停/恢复录音 | `ctrl+alt+s` |
| 取消录音 | `alt+esc` |

默认启用 `HOTKEY_HOOK`，使用 Windows 低级键盘钩子处理热键。如果热键注册失败，可以尝试以管理员权限运行，或在配置中改用其他组合。

## 配置文件

GUI 和 CLI 使用兼容的 JSON 配置格式。GUI 默认使用 `%APPDATA%\stt\config.json`，CLI 默认使用当前目录的 `config.json`，两者不会互相修改默认读取路径。

主要配置字段：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `API_ENDPOINT` | string | `""` | ASR 上传端点 URL |
| `TOKEN` | string | `""` | 授权 token |
| `MODEL` | string | `""` | 模型名称 |
| `LANGUAGE` | string | `""` | 语言 |
| `PROMPT` | string | `""` | 提示词 |
| `TEXT_PATH` | string | `"text"` | 从返回 JSON 中抽取文本的路径 |
| `ExtraConfig` | string | `""` | 字符串化 JSON，解析为根级字段并覆盖基础字段 |
| `CHANNELS` | int | `1` | 录音通道数 |
| `SAMPLING_RATE` | int | `16000` | 采样率，单位 Hz |
| `SAMPLING_RATE_DEPTH` | int | `16` | 采样位深 |
| `BIT_RATE` | int | `32` | 音频比特率，单位 kbps |
| `CODECS` | string | `"opus"` | 编码器 |
| `CONTAINER` | string | `"ogg"` | 容器格式 |
| `REQUEST_TIMEOUT` | int | `60` | 请求超时，单位秒 |
| `MAX_RETRY` | int | `3` | 上传最大重试次数 |
| `RETRY_BASE_DELAY` | float | `0.5` | 重试间隔基准，单位秒 |
| `ENABLE_HTTP2` | bool | `true` | 是否启用 HTTP/2 |
| `VERIFY_SSL` | bool | `true` | 是否验证 SSL 证书 |
| `HOTKEY_HOOK` | bool | `true` | 是否使用低级键盘钩子 |
| `START_KEY` | string | `"ctrl+alt+q"` | 开始/停止录音热键 |
| `PAUSE_KEY` | string | `"ctrl+alt+s"` | 暂停/恢复录音热键 |
| `CANCEL_KEY` | string | `"alt+esc"` | 取消录音热键 |
| `CACHE_DIR` | string | `""` | 缓存目录路径，空则使用当前目录 |
| `KEEP_CACHE` | bool | `false` | 是否保存录音、转码文件和响应 |
| `NOTIFICATION` | bool | `false` | 是否启用 Windows 通知 |
| `REQUEST_FAILED_NOTIFICATION` | bool | `false` | 请求失败后是否粘贴占位提示 |
| `FFMPEG_DEBUG` | bool | `false` | ffmpeg 调试输出 |
| `RECORD_DEBUG` | bool | `false` | 录音调试输出 |
| `HOTKEY_DEBUG` | bool | `true` | 热键调试输出 |
| `UPLOAD_DEBUG` | bool | `false` | 上传调试输出 |

`TEXT_PATH` 支持点分路径和数组索引，例如：

```text
results[0].alternatives[0].transcript
```

`ExtraConfig` 接受一个 JSON 字符串，解析后会合并到上传请求的根级字段中，适合注入服务端要求的额外参数。

## CLI 参数

命令行参数优先级高于配置文件，会覆盖配置文件中的对应设置。

| 参数 | 说明 |
|------|------|
| `-config <path>` | 指定配置文件 |
| `-file <path>` | 上传本地已有音频文件 |
| `-api-endpoint <url>` | ASR 上传端点 URL |
| `-token <token>` | 授权 token |
| `-model <model>` | 模型名称 |
| `-language <lang>` | 语言 |
| `-prompt <text>` | 提示词 |
| `-text-path <path>` | 自定义从返回 JSON 中抽取文本的路径 |
| `-extra-config <json>` | 额外 JSON 字符串，解析并合并到请求 payload |
| `-codecs` | 编码器 |
| `-container` | 容器格式 |
| `-channels` | 录音通道数 |
| `-sampling-rate` | 采样率 |
| `-sampling-rate-depth` | 采样位深 |
| `-bit-rate` | 比特率 |
| `-request-timeout` | 请求超时 |
| `-max-retry` | 最大重试次数 |
| `-retry-base-delay` | 重试基准延迟 |
| `-enable-http2` | 启用 HTTP/2 |
| `-verify-ssl` | 验证 SSL 证书 |
| `-start-key` | 开始/停止录音热键 |
| `-pause-key` | 暂停/恢复录音热键 |
| `-cancel-key` | 取消录音热键 |
| `-hotkeyhook` | 使用低级键盘钩子 |
| `-cache-dir` | 缓存目录 |
| `-keep-cache` | 保存录音与响应 |
| `-notification` | 启用通知 |
| `-request-failed-notification` | 重试耗尽后粘贴占位符 |
| `-ffmpeg-debug` | ffmpeg 调试开关 |
| `-record-debug` | 录音调试开关 |
| `-hotkey-debug` | 热键调试开关 |
| `-upload-debug` | 上传调试开关 |

## 构建

### GitHub Actions

仓库已配置 `.github/workflows/latest-release.yml`：

- 向 `main` 分支提交时自动触发。
- 也可以在 Actions 页面通过 `workflow_dispatch` 手动触发。
- 构建 Windows amd64 CLI 和 GUI。
- 发布到 `Latest` Release。
- 上传 CLI/GUI zip 以及对应 SHA256 文件。

### 本地构建 CLI

Windows 本机开发构建需要 Go、PortAudio 和 ffmpeg：

```powershell
go build -o stt.exe
```

Linux 交叉编译 Windows 版本时，需要 mingw-w64、PortAudio Windows 静态库，并设置 `CC`、`CGO_ENABLED`、`GOOS`、`GOARCH`、`PKG_CONFIG_PATH` 等环境变量。CI 中的 `.github/workflows/latest-release.yml` 可作为参考。

### 本地构建 GUI

GUI 位于 `GUI/`，使用 Wails v2：

```powershell
cd GUI
npm --prefix frontend install
npm --prefix frontend run build
wails build -platform windows/amd64
```

交叉编译 GUI 时同样需要 Windows 版 PortAudio 和 mingw-w64。GUI 构建还会通过 `scripts/build-ffmpeg-windows-amd64.sh` 编译裁剪版 FFmpeg/libav 静态库，并使用 `GOFLAGS=-tags=gui_ffmpeg_cgo` 启用内置转码实现；建议直接参考 CI 配置。

## 临时文件与缓存

- 录音阶段会创建 `RecordTemp_<uuid>.wav` 和转码后的 `RecordTemp_<uuid>.<ext>`。
- 如果配置了 `CACHE_DIR`，临时文件会写入该目录；否则使用当前工作目录。
- 程序启动时会清理当前临时目录下以 `RecordTemp_` 开头的文件。
- 启用 `KEEP_CACHE` 后，会按时间戳保留录音、转码文件和响应 JSON。

## 常见问题

- 无法初始化 PortAudio：确认 PortAudio 可用，或确认打包版本没有缺少运行时依赖。
- ffmpeg 转码失败：CLI 请确认 `ffmpeg` 在 `PATH` 中；GUI 可开启 `FFMPEG_DEBUG` 查看内置 libav 转码详情。
- 热键不可用：尝试管理员权限运行，或更换热键组合；检查是否与其他软件冲突。
- 上传失败：检查 `API_ENDPOINT`、`TOKEN`、`MODEL` 等配置；可开启 `UPLOAD_DEBUG` 查看请求与响应。
- 结果没有粘贴：确认目标应用焦点在输入框，且允许 `Ctrl+V` 粘贴。
- GUI 保存失败：录音、暂停或上传中不能保存配置，回到空闲状态后再保存。

## 安全注意

- `TOKEN` 属于敏感信息，请勿提交到公开仓库或日志中。
- `UPLOAD_DEBUG` 可能输出请求/响应内容，排查问题后建议关闭。
- 将 `VERIFY_SSL` 设为 `false` 会跳过 HTTPS 证书验证，在不受信任网络中存在风险。
