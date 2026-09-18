[English](README.md) | 简体中文

# STT for Windows

STT for Windows 是一个面向 Windows x86_64 的本地语音转文字客户端。它通过全局快捷键或原生浮窗录制麦克风音频，将完整音频文件发送到兼容的 ASR HTTP 接口，提取识别文本后自动粘贴到当前输入位置。

项目包含两个 Rust 程序：

- `STT.exe`：原生 Win32 图形界面，静态链接 PortAudio 和裁剪版 FFmpeg/libav，解压即可运行。
- `stt.exe`：命令行程序，支持快捷键录音和现有音频文件转写，与 GUI 共用内嵌 libav 转换器，无需安装系统 FFmpeg。

当前使用 Rust、Win32、Direct2D 和 DirectWrite 实现。

## 功能特性

- **原生 Windows GUI**
  - 无边框、始终置顶、支持每显示器 DPI 的浮窗。
  - Windows 10 和 Windows 11 的浮窗及设置窗口统一使用抗锯齿自绘圆角，不叠加系统外框或第二层圆角。
  - 完整模式、minimal 工具条、系统托盘、任务栏显示控制和原生设置窗口。
  - 完整模式和 minimal 模式共用可配置的透明度，以及 `0.3`–`2.0` 浮窗缩放。
  - 支持英文、简体中文、德语、日语和法语界面。
- **全局快捷键录音**
  - 开始或停止录音、暂停或恢复录音、取消录音或正在等待的识别请求。
  - 空闲且存在可重试录音时，取消或重试快捷键会重新提交该录音。
  - 默认使用低级键盘钩子，也可以改用 `RegisterHotKey`。
- **通用 ASR HTTP 接口**
  - 通过 `multipart/form-data` 上传音频，文件字段固定为 `file`。
  - 支持 Bearer Token、模型、语言、提示词和自定义表单字段。
  - 支持请求超时、指数退避重试、HTTP/2 和 TLS 证书校验。
- **可取消的处理链路**
  - 录音、内嵌 FFmpeg 转换、HTTP 上传、响应读取、重试等待和剪贴板等待均接入取消机制。
  - 请求处于 `Uploading` 状态时，GUI 保持取消按钮可用；两个程序均保持取消或重试快捷键可用。
- **录音重试**
  - GUI 和 CLI 的快捷键模式仅在进程内保存最近一条已结束录音，大小上限为 100,000,000 字节；退出程序时释放。
  - GUI 会复用取消按钮的位置显示重试；两个程序都在空闲且存在可重试录音时复用取消或重试快捷键。
- **自动提取与粘贴**
  - 使用 `TEXT_PATH` 从 JSON 响应中读取文本，支持多层对象和重复数组索引。
  - 暂存原剪贴板文本，发送 `Ctrl+V` 后再尝试恢复。
- **共享内嵌音频处理**
  - 两个程序均静态链接 libav，不搜索或启动外部 FFmpeg。
  - 可选 Earshot VAD 在 16 kHz 单声道支路检测语音，裁剪使用原始音频。
- **缓存与诊断**
  - 可选择保留原始 WAV、转码音频和成功响应。
  - 提供录音、转换、快捷键和上传调试输出。

## 下载

| 组件 | 下载 | SHA-256 |
|---|---|---|
| GUI | [stt-gui-windows-amd64.zip](https://github.com/Joey-Kot/STT-for-Windows/releases/download/Latest/stt-gui-windows-amd64.zip) | [sha256](https://github.com/Joey-Kot/STT-for-Windows/releases/download/Latest/stt-gui-windows-amd64.zip.sha256) |
| CLI | [stt-cli-windows-amd64.zip](https://github.com/Joey-Kot/STT-for-Windows/releases/download/Latest/stt-cli-windows-amd64.zip) | [sha256](https://github.com/Joey-Kot/STT-for-Windows/releases/download/Latest/stt-cli-windows-amd64.zip.sha256) |

### 应该选择哪个版本

| 使用场景 | 推荐版本 |
|---|---|
| 日常桌面使用、希望通过浮窗配置和操作 | `STT.exe` GUI |
| 自动化、脚本调用、终端快捷键录音 | `stt.exe` CLI |
| 转写已有音频并输出文本文件 | `stt.exe` CLI |
| 不想安装 FFmpeg | GUI 或 CLI |

## 架构

`stt-core` 负责配置、录音、状态机、ASR、缓存、快捷键和剪贴板。GUI 与 CLI 只提供不同的交互方式，共用音频处理实现。

```mermaid
flowchart LR
    subgraph Entry["控制入口"]
        GUI["STT.exe<br/>Win32 GUI"]
        CLI["stt.exe<br/>快捷键模式"]
        FileMode["stt.exe --file<br/>文件模式"]
    end

    GUI --> Runtime["stt-core<br/>运行时状态机"]
    CLI --> Runtime
    FileMode --> FilePipeline["文件转写流程"]

    Runtime --> Recorder["PortAudio<br/>麦克风录音"]
    Recorder --> WAV["PCM 16-bit WAV"]

    WAV --> Convert["录音音频转换抽象"]
    WAV --> RetryBuffer["GUI 和 CLI 重试缓冲<br/>最近一条已结束 WAV，仅内存，≤100 MB"]
    RetryBuffer -->|重试| Convert
    Convert --> LibAv["共享内嵌 libav + 可选 Earshot VAD"]
    FilePipeline --> LibAv

    LibAv --> Request["ASR multipart 请求"]
    Request --> Extract["JSON + TEXT_PATH"]

    Extract -->|快捷键/GUI 模式| Channel{USE_SENDINPUT}
    Channel -->|false，默认| Clipboard["CF_UNICODETEXT<br/>Ctrl+V + 恢复"]
    Channel -->|true| Unicode["SendInput Unicode<br/>不使用剪贴板，无回退"]
    Clipboard --> App["当前前台应用"]
    Unicode --> App
    Extract -->|文件模式| TextFile["文本文件"]
```

GUI 和 CLI 共用相同的配置格式与 ASR 请求语义。两者的主要差别是界面和配置文件位置。

## 录音与识别流程

```mermaid
sequenceDiagram
    actor User as 用户
    participant Control as GUI / 全局快捷键
    participant Runtime as Rust 状态机
    participant Recorder as PortAudio
    participant Converter as 内嵌 libav
    participant ASR as ASR HTTP API
    participant Clipboard as Windows 剪贴板
    participant App as 当前前台应用

    User->>Control: 开始
    Control->>Runtime: toggle recording
    Runtime->>Recorder: 初始化设备并创建 WAV
    Recorder-->>Runtime: Recording

    opt 暂停与恢复
        User->>Control: 暂停 / 恢复
        Control->>Runtime: toggle pause
        Runtime->>Recorder: 停止或继续读取音频
    end

    User->>Control: 停止
    Control->>Runtime: toggle recording
    Runtime->>Recorder: 停止并完成 WAV
    Recorder-->>Runtime: RecordingResult
    Runtime->>Runtime: 若 WAV ≤100 MB，则保留最近一条于内存
    Runtime->>Runtime: 进入 Uploading
    Runtime->>Converter: 转换到配置的编码与容器
    Converter-->>Runtime: 转码音频
    Runtime->>ASR: multipart/form-data POST

    alt 手动取消
        User->>Control: 取消按钮 / 取消或重试快捷键
        Control->>Runtime: 取消当前请求令牌
        Runtime-->>ASR: 中止上传、响应或重试等待
        Runtime-->>Control: Idle / 请求已取消
    else HTTP 200
        ASR-->>Runtime: JSON 响应
        Runtime->>Runtime: 按 TEXT_PATH 提取文本
        Note over Runtime,App: 通过 core 统一入口输出；输入期间仍处于 Uploading
        alt USE_SENDINPUT=true
            Runtime->>Runtime: 等待修饰键释放，按 UTF-16 分批发送
            Runtime->>App: SendInput Unicode（不使用剪贴板，无回退）
            Note over Runtime,App: 批次间检查取消；已注入事件无法撤回
        else 默认剪贴板通道
            Runtime->>Clipboard: 保存原文本并写入识别结果
            Clipboard->>App: keybd_event 发送 Ctrl+V
            Runtime->>Clipboard: 恢复原剪贴板文本
        end
        alt 输入成功
            Runtime-->>Control: Idle / 已粘贴或文本输入已发送
        else 剪贴板操作取消或 SendInput 尚未发送时取消
            Runtime-->>Control: Idle / 请求已取消
        else 输入失败、部分发送、发送后取消或剪贴板恢复失败
            Note over Runtime,Control: 显示具体错误；部分发送提示可能已有文本，不自动重发
            alt 有重试缓冲
                Runtime-->>Control: Idle / 可手动重试
            else 无重试缓冲
                Runtime-->>Control: Error
            end
        end
    else 请求最终失败
        ASR-->>Runtime: 非 200 或网络错误
        opt 重试耗尽且启用请求失败提示
            Runtime->>App: 经所选通道输出 [request failed]
        end
        alt 已取消
            Runtime-->>Control: Idle / 请求已取消
        else 有重试缓冲
            Runtime-->>Control: Idle / 可手动重试
        else 无重试缓冲
            Runtime-->>Control: Error
        end
    end

    opt 存在可重试录音
        User->>Control: 点击重试图标或按取消或重试快捷键
        Control->>Runtime: 临时还原缓冲 WAV 并重试
    end
```

系统不会边录音边流式上传。只有停止录音并完成 WAV 后，才会进行转码和 ASR 请求。完成的 WAV 超过 100,000,000 字节时，请求仍会正常进行，但 GUI 和 CLI 的快捷键模式不会保留它用于重试；此时处理最终失败会进入 `Error`。

## 运行时状态机

```mermaid
stateDiagram-v2
    [*] --> Idle

    Idle --> Recording: 开始
    Error --> Recording: 重新开始

    Recording --> Paused: 暂停
    Paused --> Recording: 恢复

    Recording --> Uploading: 停止并完成 WAV
    Paused --> Uploading: 停止并完成 WAV

    Idle --> Uploading: 重试缓冲的 WAV

    Recording --> Idle: 取消录音
    Paused --> Idle: 取消录音

    Uploading --> Idle: 剪贴板粘贴成功或 SendInput 发送完成
    Uploading --> Idle: 识别结果为空
    Uploading --> Idle: 转换或请求取消、剪贴板操作取消、SendInput 发送前取消
    Uploading --> Idle: 处理失败且有重试缓冲
    Uploading --> Error: 处理失败且无重试缓冲

    note right of Uploading
        包含转换、ASR 请求和文本输出
        处理失败包括转换或上传失败、输入失败、
        剪贴板恢复失败、SendInput 部分发送或发送后取消
        部分发送或发送后取消提示可能已有文本
        无自动通道回退或文本重发
    end note

    Error --> Idle: 保存有效设置
```

普通动作使用一个非排队动作锁。繁忙时重复的开始、停止或暂停动作会被丢弃，不会排队到稍后执行。`Uploading` 状态下的取消是例外：它绕过动作锁，直接取消当前请求令牌。

只有结束一段新的录音才会替换重试缓冲。录制中取消会保留上一条缓冲录音；上传中取消会保留刚取消请求的录音。重试成功或失败后，仍保留同一条缓冲录音。

## 功能范围与当前限制

- 当前 Release 只提供 Windows x86_64 构建。
- GUI 是 Windows 专用原生程序；CLI 源码可在其他系统编译，但全局 Windows 快捷键功能只在 Windows 可用。
- 录音使用系统默认输入设备，目前没有麦克风设备选择器。
- 录音后一次性上传完整音频，不支持实时流式识别。
- ASR 接口必须接受 `multipart/form-data` 并返回 JSON。
- 只有 HTTP 200 被视为成功；其他状态码进入重试或失败流程。
- HTTP 客户端不使用系统代理、不自动跟随重定向，也不启用自动响应压缩。
- 内嵌 libav 在阻塞工作线程运行，解码、区间处理和文件 I/O 均接入取消回调；清理会等待工作线程关闭输出。
- 自动粘贴发送到识别完成时的前台应用。用户在等待期间切换焦点，会改变最终粘贴目标。
- GUI 不提供 Windows Toast、托盘气泡或其他系统通知。
- 旧配置中的 `NOTIFICATION` 会被忽略，保存时不会重新写入。
- `REQUEST_FAILED_NOTIFICATION` 不是系统通知开关；它只控制重试耗尽后是否粘贴 `[request failed]`。

## 运行要求

### GUI

- Windows 10 或 Windows 11 x86_64。
- 可用的默认麦克风输入设备。
- 一个兼容的 ASR HTTP 接口。
- 无需安装 FFmpeg、PortAudio、WebView2 或 Visual C++ Redistributable。

### CLI

- Windows x86_64。
- 快捷键录音模式需要麦克风。
- 无需安装系统 FFmpeg。
- 一个兼容的 ASR HTTP 接口。

### 从源码开发

- Rust 1.97 或更新版本。
- `x86_64-pc-windows-gnu` Rust 目标。
- MinGW-w64、C/C++ 构建工具、`pkg-config`、Autoconf、Automake、Libtool、NASM、YASM 和 XZ 工具。
- 构建静态音频依赖时需要能够获取 PortAudio、FFmpeg 和编解码器源码。

## GUI 使用

### 首次启动

1. 下载并解压 `stt-gui-windows-amd64.zip`。
2. 运行 `STT.exe`。
3. 程序会在以下位置创建默认配置：

```text
%APPDATA%\stt\config.json
```

4. 通过浮窗齿轮按钮或托盘菜单打开设置。
5. 至少填写 `API_ENDPOINT`，并按服务要求填写 `TOKEN`、`MODEL` 和 `TEXT_PATH`。
6. 保存设置后使用浮窗按钮或默认快捷键开始录音。

界面语言单独保存在：

```text
%APPDATA%\stt\ui-language.txt
```

语言设置不会写入 ASR 配置文件，也不会改变请求中的 `LANGUAGE` 字段。

### 浮窗操作

| 控件 | 可用状态 | 行为 |
|---|---|---|
| 麦克风 | `Idle`、`Error`、`Recording`、`Paused` | 开始录音，或停止录音并进入识别流程 |
| 暂停/播放 | `Recording`、`Paused` | 暂停或恢复录音 |
| 取消 / 重试 | `Recording`、`Paused`、`Uploading`；存在可重试录音时的 `Idle` | 取消当前录音或正在等待的识别请求。`Idle` 且无可重试录音时，该位置仍显示不可用的取消图标；存在可重试录音时显示重试图标，并重新提交缓冲录音 |
| 齿轮 | 任意非关闭状态 | 打开原生设置窗口 |
| `-` / `+` | 任意状态 | 切换完整浮窗与 minimal 工具条 |
| 顶部拖动条 | 完整模式 | 移动浮窗 |
| 工具条空白区域或按钮拖动 | minimal 模式 | 移动工具条；超过拖动阈值时不会触发按钮动作 |

完整模式显示在任务栏；minimal 模式隐藏任务栏标签，但保留托盘图标。托盘菜单包含 `Minimal`、`Settings` 和 `Quit`，双击托盘图标会恢复完整模式。Display 页的浮窗缩放在保存后立即生效，同时缩放两种模式的窗口、绘制内容和鼠标命中区域。

### 设置窗口

| 页面 | 内容 |
|---|---|
| Display | 界面语言、配置文件位置、浮窗透明度和浮窗缩放 |
| API | 地址、Token、模型、语言、提示词、文本路径和额外字段 |
| Audio | 声道、输出采样率、采样位深、比特率、编码器、容器、VAD 和边界填充 |
| Network | 超时、重试、HTTP/2 和 TLS 校验 |
| Hotkeys | 三个快捷键、低级键盘钩子开关、两个剪贴板等待时间和使用 SendInput 开关 |
| Cache | 缓存目录、缓存保留和请求失败占位文本 |
| Debug | FFmpeg、录音、快捷键和上传调试 |
| About | 项目、作者、许可证和仓库信息 |

只有 `Idle` 或 `Error` 状态允许保存设置。保存时程序会验证配置，重建 ASR 客户端和录音器，并重新注册快捷键。

### 退出

- 按 `Esc` 时优先关闭设置窗口；设置窗口未打开时会进入退出流程。
- 录音、暂停或上传期间退出会显示确认对话框。
- 退出会取消录音和当前请求、移除托盘图标并停止快捷键线程。

## 命令行程序

`stt.exe` 支持两种模式：

- **快捷键模式**：常驻终端，通过全局快捷键录音、识别和粘贴。
- **文件模式**：转写已有音频文件，将文本写入指定文件。

### 配置查找与优先级

配置优先级为：

```text
命令行覆盖参数 > --config 指定的 JSON > 当前目录 config.json > 默认值
```

如果没有 `--config`、当前目录不存在 `config.json`，并且没有提供任何配置覆盖参数，CLI 会创建默认 `config.json`、打印路径并退出。编辑后重新运行即可。

所有长参数都使用标准双横线形式。布尔参数必须显式传入 `true` 或 `false`。旧式单横线长参数和已删除的 `--notification` 不受支持。

### 快捷键模式

使用当前目录配置：

```powershell
.\stt.exe
```

指定配置文件：

```powershell
.\stt.exe --config .\config.json
```

完全使用命令行覆盖：

```powershell
.\stt.exe `
  --api-endpoint "https://api.example.com/v1/audio/transcriptions" `
  --token "your-token" `
  --model "your-model" `
  --text-path "text"
```

启动后程序会在终端打印状态变化。按 `Ctrl+C` 退出。

### 文件模式

```powershell
.\stt.exe `
  --config .\config.json `
  --file .\sample.wav `
  --output .\sample.txt
```

如果省略 `--output`，输出默认为当前目录下的 `<输入文件名>.txt`。文件模式会先按音频配置转换输入文件，再上传识别；它不会注册全局快捷键，也不会自动粘贴。

### CLI 参数

`--help` 会按照与 GUI 设置页对应的参数组显示选项。

#### General

| 参数 | 用途 |
|---|---|
| `--config <PATH>` | 指定 JSON 配置文件 |
| `--file <PATH>` | 进入文件模式并指定已有音频 |
| `--output <PATH>` | 文件模式的文本输出路径 |

#### API

| 参数 | 用途 |
|---|---|
| `--api-endpoint <URL>` | 覆盖 ASR 接口地址 |
| `--token <TOKEN>` | 覆盖 Bearer Token |
| `--model <MODEL>` | 覆盖模型字段 |
| `--language <LANGUAGE>` | 覆盖请求语言字段 |
| `--prompt <TEXT>` | 覆盖提示词 |
| `--text-path <PATH>` | 覆盖响应文本路径 |
| `--extra-config <JSON>` | 覆盖字符串化额外 JSON 对象 |

#### Audio

| 参数 | 用途 |
|---|---|
| `--codecs <CODEC>` | 覆盖音频编码器 |
| `--container <FORMAT>` | 覆盖音频容器 |
| `--channels <N>` | 覆盖声道数 |
| `--sampling-rate <HZ>` | 覆盖最终上传采样率；`--rate` 是兼容别名 |
| `--sampling-rate-depth <BITS>` | 覆盖转换采样位深 |
| `--bit-rate <KBPS>` | 覆盖音频比特率 |
| `--enable-vad <BOOL>` | 开启或显式关闭语音裁剪，默认 false |
| `--vad-padding-ms <0-1000>` | 边界填充毫秒数，默认 100 |

#### Network

| 参数 | 用途 |
|---|---|
| `--request-timeout <SECONDS>` | 覆盖单次客户端请求超时 |
| `--max-retry <N>` | 覆盖最大请求次数 |
| `--retry-base-delay <SECONDS>` | 覆盖指数退避初始等待 |
| `--enable-http2 <BOOL>` | 启用或禁用 HTTP/2 |
| `--verify-ssl <BOOL>` | 启用或禁用 TLS 证书校验 |

#### Hotkeys

| 参数 | 用途 |
|---|---|
| `--start-key <HOTKEY>` | 覆盖开始/停止快捷键 |
| `--pause-key <HOTKEY>` | 覆盖暂停/恢复快捷键 |
| `--cancel-or-retry-key <HOTKEY>` | 覆盖取消录音/请求或重试最近一条已结束录音的快捷键 |
| `--hotkey-hook <BOOL>` | 选择低级键盘钩子或 `RegisterHotKey` |
| `--clipboard-write-delay <MS>` | 覆盖写入识别文本后、发送 `Ctrl+V` 前的等待时间 |
| `--clipboard-restore-delay <MS>` | 覆盖发送 `Ctrl+V` 后、恢复原剪贴板前的等待时间 |
| `--use-sendinput <BOOL>` | 覆盖 `USE_SENDINPUT`；直接输入 Unicode 文本，不使用剪贴板，无回退 |

#### Cache

| 参数 | 用途 |
|---|---|
| `--cache-dir <PATH>` | 覆盖缓存目录 |
| `--keep-cache <BOOL>` | 控制是否保留缓存 |
| `--request-failed-notification <BOOL>` | 控制失败后是否粘贴 `[request failed]` |

#### Debug

| 参数 | 用途 |
|---|---|
| `--ffmpeg-debug <BOOL>` | FFmpeg 调试输出 |
| `--record-debug <BOOL>` | 录音调试输出 |
| `--hotkey-debug <BOOL>` | 快捷键调试输出 |
| `--upload-debug <BOOL>` | 上传调试输出 |

`--help` 显示完整帮助，`--version` 显示版本。

参数解析失败由 Clap 返回退出码 `2`；运行时、请求、转换或文件错误返回退出码 `1`；成功、未检测到语音或首次生成默认配置返回 `0`。文件模式也可通过 Ctrl+C 取消分析、转换和上传。

## 配置文件

GUI 和 CLI 使用相同的 JSON 数据结构。缺失字段自动使用默认值，未知字段会被忽略。

### OpenAI 兼容接口示例

```json
{
  "API_ENDPOINT": "https://api.openai.com/v1/audio/transcriptions",
  "TOKEN": "sk-xxx",
  "MODEL": "gpt-4o-mini-transcribe",
  "LANGUAGE": "zh",
  "PROMPT": "",
  "TEXT_PATH": "text",
  "ExtraConfig": "{\"response_format\":\"json\",\"temperature\":0}",
  "OPACITY": 1.0,
  "WINDOW_SCALE": 1.0,
  "CHANNELS": 1,
  "SAMPLING_RATE": 16000,
  "ENABLE_VAD": false,
  "VAD_PADDING_MS": 100,
  "SAMPLING_RATE_DEPTH": 16,
  "BIT_RATE": 128,
  "CODECS": "mp3",
  "CONTAINER": "mp3",
  "REQUEST_TIMEOUT": 300,
  "MAX_RETRY": 3,
  "RETRY_BASE_DELAY": 0.5,
  "ENABLE_HTTP2": true,
  "VERIFY_SSL": true,
  "HOTKEY_HOOK": true,
  "START_KEY": "ctrl+alt+q",
  "PAUSE_KEY": "ctrl+alt+s",
  "CANCEL_OR_RETRY_KEY": "alt+esc",
  "CLIPBOARD_WRITE_DELAY": 80,
  "CLIPBOARD_RESTORE_DELAY": 120,
  "CACHE_DIR": "",
  "KEEP_CACHE": false,
  "REQUEST_FAILED_NOTIFICATION": false,
  "FFMPEG_DEBUG": false,
  "RECORD_DEBUG": false,
  "HOTKEY_DEBUG": false,
  "UPLOAD_DEBUG": false
}
```

这只是协议示例。实际模型名、字段、支持的音频格式和超时应以所使用的 ASR 服务为准。正常公网服务应保持 `VERIFY_SSL=true`。

### 显示字段

| 字段 | 默认值 | 行为 |
|---|---:|---|
| `OPACITY` | `1.0` | GUI 浮窗透明度。允许 `0.10`–`1.00`，步进 `0.01`；`1.0` 为完全不透明。完整模式和 minimal 模式共用此设置。 |
| `WINDOW_SCALE` | `1.0` | GUI 浮窗缩放。允许 `0.3`–`2.0`，步进 `0.1`；保存后立即应用于完整模式和 minimal 模式的窗口、绘制内容及鼠标命中区域。 |

### API 与响应字段

| 字段 | 默认值 | 行为 |
|---|---:|---|
| `API_ENDPOINT` | `""` | ASR POST 地址；上传时不能为空 |
| `TOKEN` | `""` | 非空时发送 `Authorization: Bearer <token>` |
| `MODEL` | `""` | 非空时发送 multipart 字段 `model` |
| `LANGUAGE` | `""` | 非空时发送 multipart 字段 `language` |
| `PROMPT` | `""` | 非空时发送 multipart 字段 `prompt` |
| `TEXT_PATH` | `"text"` | 从 JSON 响应读取识别文本 |
| `ExtraConfig` | `""` | 字符串化 JSON 对象，用于增删或覆盖 multipart 字段 |

### 音频字段

| 字段 | 默认值 | 验证与行为 |
|---|---:|---|
| `CHANNELS` | `1` | 允许 1–8；录音和转换都使用该值 |
| `SAMPLING_RATE` | `16000` | 最终上传采样率，必须大于 0，单位 Hz |
| `SAMPLING_RATE_DEPTH` | `16` | 允许 8、16、24、32；只影响转换，不改变录音 WAV 的 PCM 16-bit 格式 |
| `BIT_RATE` | `32` | 必须大于 0，单位 kbps |
| `CODECS` | `"opus"` | 编码器名称或兼容别名，大小写不敏感 |
| `CONTAINER` | `"ogg"` | 输出容器/扩展名，大小写不敏感 |

共享静态构建覆盖的常用输出包括 Opus/Ogg、MP3、AAC、FLAC、Vorbis 和 WAV/PCM。构建中还包含部分其他编码器与封装器；编码器和容器必须是有效组合。

### 语音检测与捕获

麦克风优先以 48 kHz PCM 16-bit 捕获，失败后依次尝试设备默认采样率、配置的输出采样率。WAV 头保存实际捕获采样率。`SAMPLING_RATE` 只控制最终上传采样率。

| 字段 | 默认值 | 行为 |
|---|---:|---|
| `ENABLE_VAD` | `false` | 适用于 GUI 录音、CLI 录音和 CLI `--file` |
| `VAD_PADDING_MS` | `100` | 整数 0～1000 ms；VAD 关闭时仍保留并校验 |

Audio 页关闭 VAD 后，边界填充输入框置灰并保留原值。Earshot 1.2.2 在流式 16 kHz 单声道 PCM 上检测，只输出语音区间；最终裁剪、拼接、重采样和编码始终基于原始音频。不生成分析 WAV 或裁剪中间文件，不使用 libavfilter、大型 filtergraph 或固定区间数量上限。

首个语音片段之前、最后一个片段之后最多各保留完整 padding。内部拼接处，前段后方保留 floor(padding/2) 毫秒，后段前方保留剩余部分，总计一个 padding。原间隔不超过 padding 时完整保留并合并；padding 为 0 时直接拼接语音边界。

未检测到语音时不请求 ASR、不生成文本文件。GUI/快捷键模式返回 Idle，提示“未检测到语音”并清除重试任务；CLI 文件模式打印结果后正常退出。临时文件会被清理。检测到语音时，重试缓冲仍保留原始高质量 WAV；手动重试，以及开启 VAD 时的自动 HTTP 重试，都会重新检测、裁剪和转码。关闭 VAD 时保持原有 HTTP 重试行为。`KEEP_CACHE` 按原有规则保存原始音频、最终转换音频和成功响应。

内嵌构建支持 WAV/PCM、MP3、FLAC、Ogg/Opus、Ogg/Vorbis、M4A/MP4/AAC、M4A/ALAC、WebM/Matroska 音频、WavPack、AC3/EAC3。无法解码的流会明确报错，不回退到外部程序。

### 网络字段

| 字段 | 默认值 | 行为 |
|---|---:|---|
| `REQUEST_TIMEOUT` | `60` | 大于 0 时设置 reqwest 客户端超时，单位秒；非正值表示不主动设置 |
| `MAX_RETRY` | `3` | 最大请求次数，包含第一次请求 |
| `RETRY_BASE_DELAY` | `0.5` | 第一次重试前等待秒数，之后每次翻倍 |
| `ENABLE_HTTP2` | `true` | 为 `false` 时强制 HTTP/1 |
| `VERIFY_SSL` | `true` | 为 `false` 时接受无效 TLS 证书，不建议用于公网 |

### 快捷键、剪贴板、缓存与调试字段

| 字段 | 默认值 | 行为 |
|---|---:|---|
| `HOTKEY_HOOK` | `true` | `true` 使用 `WH_KEYBOARD_LL`；`false` 使用 `RegisterHotKey` |
| `START_KEY` | `"ctrl+alt+q"` | 开始或停止录音 |
| `PAUSE_KEY` | `"ctrl+alt+s"` | 暂停或恢复录音 |
| `CANCEL_OR_RETRY_KEY` | `"alt+esc"` | 取消录音或当前识别请求；空闲且存在可重试录音时重试 |
| `CLIPBOARD_WRITE_DELAY` | `80` | 写入识别文本后、发送 `Ctrl+V` 前的等待时间，单位毫秒 |
| `CLIPBOARD_RESTORE_DELAY` | `120` | 发送 `Ctrl+V` 后、恢复原剪贴板前的等待时间，单位毫秒 |
| `USE_SENDINPUT` | `false` | GUI 和 CLI 快捷键模式使用 core 的 Unicode 直接输入通道 |
| `CACHE_DIR` | `""` | 非空时尝试创建并转换为绝对路径；失败时回退当前目录并清空设置值 |
| `KEEP_CACHE` | `false` | 只有 `CACHE_DIR` 非空且可用时才保留缓存 |
| `REQUEST_FAILED_NOTIFICATION` | `false` | 重试耗尽后粘贴 `[request failed]`；不会发送系统通知 |
| `FFMPEG_DEBUG` | `false` | 打印转换后端信息 |
| `RECORD_DEBUG` | `false` | 打印录音诊断信息 |
| `HOTKEY_DEBUG` | `true` | 打印快捷键注册和繁忙动作信息 |
| `UPLOAD_DEBUG` | `false` | 打印上传目标、尝试次数和失败响应摘要 |

## ASR 接口兼容要求

程序发送一个 HTTP POST 请求：

```http
POST <API_ENDPOINT>
User-Agent: stt-go-client/1.0
Content-Type: multipart/form-data; boundary=<自动生成>
```

`TOKEN` 非空时，客户端还会发送 `Authorization: Bearer <TOKEN>`。multipart 的 `boundary` 参数由客户端为每次请求自动生成，不应在服务端配置中手工固定。

multipart 内容：

| 字段 | 发送条件 |
|---|---|
| `file` | 始终发送；内容为转换后的音频，文件名使用本地文件名 |
| `model` | `MODEL` 非空 |
| `language` | `LANGUAGE` 非空 |
| `prompt` | `PROMPT` 非空 |
| 其他字段 | 来自 `ExtraConfig` |

每次重试都会重新打开音频文件并重建 multipart 请求体。系统代理、自动重定向和自动 gzip/brotli/deflate 解压都被禁用。

### ExtraConfig

`ExtraConfig` 本身是 JSON 字符串，其中的内容必须是一个 JSON 对象：

```json
{
  "ExtraConfig": "{\"response_format\":\"json\",\"temperature\":0,\"stream\":false}"
}
```

合并规则：

- 字符串、布尔值和数字转换为普通表单文本。
- 对象和数组序列化为紧凑 JSON 字符串。
- 同名字段覆盖 `model`、`language` 或 `prompt`。
- 值为 `null` 时删除对应的内置字段。
- 合并是浅层合并，不进行递归对象合并。

例如，以下配置会删除 `language` 并覆盖 `model`：

```json
{
  "ExtraConfig": "{\"language\":null,\"model\":\"custom-model\"}"
}
```

### TEXT_PATH

`TEXT_PATH` 使用点号分隔对象字段，并允许一个字段后跟任意数量的数组索引：

```text
text
result.transcript
results[0].alternatives[0].transcript
data.items[0][1].text
```

字符串、数字和布尔值都会转换为文本。配置路径无法读取时，程序依次尝试：

1. 顶层 `text`。
2. 顶层第一个非空字符串字段。
3. 返回空字符串。

非 JSON 响应无法提取文本。HTTP 200 但结果为空时，状态返回 `Idle`，不会粘贴占位内容。

### 重试与取消

- 请求错误和非 200 响应会进入重试流程。
- `MAX_RETRY` 包含首次请求。
- 等待时间从 `RETRY_BASE_DELAY` 开始，每次失败后乘以 2。
- 手动取消会中止正在进行的请求发送、响应读取或重试等待。
- 取消不是错误：GUI 和 CLI 的快捷键模式会返回 `Idle` 并显示“请求已取消”。
- 只有重试耗尽且 `REQUEST_FAILED_NOTIFICATION=true` 时，才会尝试粘贴 `[request failed]`。
- GUI 和 CLI 的快捷键模式会把最近一条已结束的录音作为可重试 WAV 保存在内存中，前提是大小不超过 100,000,000 字节。手动取消请求，以及重试成功或失败后，都会保留该 WAV。
- 取消录制不会替换上一条可重试 WAV。结束一段新录音会替换它；新录音超过上限时不保留可重试 WAV。

## 默认快捷键与语法

| 动作 | 默认快捷键 |
|---|---|
| 开始/停止录音 | `ctrl+alt+q` |
| 暂停/恢复录音 | `ctrl+alt+s` |
| 取消录音/识别请求，或在空闲时重试最近一条已结束录音 | `alt+esc` |

支持的修饰键别名：

- `alt`、`menu`
- `ctrl`、`control`
- `shift`
- `win`、`meta`、`super`

支持字母、数字、`F1`–`F24`、方向键、`Esc`、`Space`、`Enter`、`Tab`、`Backspace`、`Insert`、`Delete`、`Home`、`End`、`PageUp`、`PageDown` 和数字键盘别名。

快捷键不区分大小写，重复修饰键、未知按键以及三个动作之间的等价重复绑定都会被拒绝。

`HOTKEY_HOOK=true` 时使用低级键盘钩子：

- 忽略注入的键盘事件。
- 抑制按住快捷键产生的重复触发。
- 只要求配置的修饰键已按下，不禁止额外修饰键。

`HOTKEY_HOOK=false` 时使用 `RegisterHotKey` 和 `MOD_NOREPEAT`。

## 剪贴板与自动粘贴

默认情况下（`USE_SENDINPUT=false`），Windows GUI 和快捷键模式使用 `CF_UNICODETEXT`：

1. 读取并保存当前剪贴板文本。
2. 写入识别结果。
3. 等待 `CLIPBOARD_WRITE_DELAY` 毫秒，默认值为 80。
4. 通过 `keybd_event` 发送 `Ctrl+V`。
5. 等待 `CLIPBOARD_RESTORE_DELAY` 毫秒，默认值为 120。
6. 无论前面是否成功，都尝试恢复原剪贴板文本。

两个等待时间位于 GUI 的 `Hotkeys` 页面，也可以通过同名 JSON 字段或 CLI 的 `Hotkeys` 参数组设置。配置中缺少字段时仍使用 80 ms 和 120 ms。

如果粘贴快捷键已经发送，但恢复原剪贴板失败，程序会把它与“粘贴前失败”区分显示。

在 Hotkeys 页面恢复等待项下方开启“使用 SendInput”，或设置 `USE_SENDINPUT=true`、传入 `--use-sendinput true`，即可直接输入 Unicode 文本。旧配置缺少字段时默认关闭。识别结果、用户主动重试后的结果和 `[request failed]` 提示均遵守此设置；标准输出和文件输出不受影响。开启时两个剪贴板等待项置灰，保留原值。

新通道不读写剪贴板，不自动回退或重发。文本按 UTF-16 分批发送，批次不会拆开代理对。CRLF 和 LF 统一为 CR；换行和 Tab 发送 Unicode 字符事件，不模拟物理 Enter/Tab 按键，实际效果仍取决于目标控件。修饰键未释放时最多等待两秒；取消停止后续批次，已输入内容无法撤回。部分发送会明确提示可能已有文本。API 成功表示事件已注入，不代表目标控件已接收；输入焦点、控件兼容性和 Windows 权限限制仍然适用。

## 缓存与临时文件

启动时，程序会清理活动临时目录中所有以 `RecordTemp_` 开头的文件或目录。

临时录音名：

```text
RecordTemp_<16 位十六进制字符>.wav
```

转换文件沿用相同基础名并使用配置的容器扩展名。当输入和输出都是 WAV 时，转换文件增加 `_convert`，避免覆盖原始录音。

当 `KEEP_CACHE=false` 或 `CACHE_DIR` 为空时，流程结束后删除临时音频。缓存启用时，文件重命名为：

```text
audio-YYYY-MM-DD-HH.MM.SS.<ext>
```

只有 HTTP 200 的响应会写入对应的 `.json` 文件。失败或在收到成功响应前取消时，不会生成响应 JSON。

GUI 和 CLI 快捷键模式的重试缓冲独立于这里的可选磁盘缓存：它只在内存中保留最近一条已结束 WAV，最大 100,000,000 字节，并会在进程退出时释放（包括正常关闭、注销或断电）。重试时会临时还原一个 `RecordTemp_` WAV 以供转换，并在本次尝试后删除；不会创建持久化重试缓存。`KEEP_CACHE` 仍只控制普通录音请求原有的可选音频归档。

## 从源码构建

正式发布使用 Ubuntu、MinGW-w64 和 Rust `x86_64-pc-windows-gnu` 目标交叉构建。

### 安装 Rust 目标

```bash
rustup target add x86_64-pc-windows-gnu
rustup component add rustfmt clippy
```

### 测试与静态检查

```bash
cargo fmt --all --check
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo check --workspace \
  --target x86_64-pc-windows-gnu \
  --features stt-gui/native-gui
```

### 构建原生依赖与程序

```bash
scripts/build-portaudio-windows-amd64.sh
scripts/build-ffmpeg-windows-amd64.sh
scripts/build-rust-windows-amd64.sh
scripts/package-windows-release.sh
```

构建结果：

```text
dist/cli/stt.exe
dist/gui/STT.exe
dist/stt-cli-windows-amd64.zip
dist/stt-gui-windows-amd64.zip
```

`scripts/build-portaudio-windows-amd64.sh` 构建 PortAudio WMME 后端，不启用 WASAPI。`scripts/build-ffmpeg-windows-amd64.sh` 使用 `--disable-everything` 后启用 file 协议；wav/mp3/flac/ogg/mov/aac/matroska/wv/ac3/eac3 解封装器；对应的 PCM、MP3/MP2、FLAC、Opus、Vorbis、AAC、ALAC、WavPack、AC3/EAC3 解码器与解析器；以及原有输出编码器和封装器。禁用 libavfilter。两个程序均启用共享的 `stt-core/static-libav` 功能，Earshot 固定为 1.2.2。

GitHub Actions 还会检查：

- 格式、测试和 `clippy -D warnings`。
- Windows API 与 MinGW 目标编译。
- PortAudio 后端必须包含 WMME 且不包含 WASAPI。
- FFmpeg 构建不得启用 `nonfree`。
- CLI 和 GUI 均包含 `keybd_event` 和 `SendInput`，支持两种可选输入通道。
- GUI 不得包含外部 FFmpeg 后端。
- GUI 不得动态依赖 PortAudio 或 libav DLL。
- `NOTICE` 与 `THIRD_PARTY_LICENSES/` 必须完整。

构建通过后，工作流会更新 `Latest` 标签和 Release，并上传 GUI、CLI 及其 SHA-256 文件。

## 安全与隐私

- 录音和转码默认在本机完成，只有转换后的音频会发送到 `API_ENDPOINT`。
- `TOKEN` 以明文保存在 JSON 配置中。GUI 的密码输入框只负责遮挡显示，不提供磁盘加密。
- 对公网服务应保持 `VERIFY_SSL=true`。
- `VERIFY_SSL=false` 会接受无效证书，可能遭受中间人攻击。
- HTTP 客户端不会读取系统代理设置。如需代理，应在可信网关或 API 端处理。
- 程序不会验证所配置 API 是否可信；请只使用你愿意发送录音内容的服务。
- `CACHE_DIR` 中可能包含原始录音、转码音频和服务响应，应按敏感数据管理。
- 自动粘贴依赖当前前台窗口，开始录音后不要在不希望接收文本的窗口中保留输入焦点。

## 实现约束

- 录音：PortAudio C 阻塞 API、默认输入设备、WMME；不使用 WASAPI。
- 录音格式：交错 signed int16，临时 WAV 始终为 PCM 16-bit。
- GUI 转换：静态 libav C ABI；不启动 `ffmpeg.exe`。
- CLI 转换：与 GUI 共用内嵌 libav 转换器及取消回调。
- GUI：Win32 消息循环、Direct2D、DirectWrite 和原生控件；不嵌入 WebView。
- 托盘：`Shell_NotifyIconW`；不发送托盘气泡。
- 默认粘贴：`keybd_event`；可选 Unicode 直接输入：`SendInput`。
- 通知：不提供 Windows 系统通知。
- 配置：保存前验证，缺失字段使用默认值，未知字段忽略。

更精确的兼容行为见 [Rust 重写兼容合同](docs/rust-rewrite-contract.md)，自动与人工验证边界见 [Rust 技术验证记录](docs/rust-technical-validation.md)。

## 仓库布局

| 组件 | 路径 | 作用 / 输出 |
|---|---|---|
| 核心库 | `crates/stt-core/` | 配置、ASR、缓存、录音、快捷键、剪贴板和状态机 |
| CLI | `crates/stt-cli/` | `stt.exe` |
| 原生 GUI | `crates/stt-gui/` | `STT.exe` |
| libav 桥接 | `native/` | GUI 与 CLI 共用的 C ABI |
| 构建脚本 | `scripts/` | PortAudio、FFmpeg、Rust 和发布包构建 |
| Windows 资源 | `assets/` | 程序图标等资源 |
| 示例配置 | `examples/` | 服务商配置示例 |
| 行为与验证文档 | `docs/` | Rust 兼容合同和技术验证记录 |
| 发布工作流 | `.github/workflows/latest-release.yml` | 构建并更新 `Latest` Release |

## 第三方组件

两个发布包均静态链接：

- FFmpeg/libav n7.1.1
- PortAudio v19.7.0
- Opus v1.5.2
- LAME 3.100
- libogg 1.3.5
- libvorbis 1.3.7
- OpenCore AMR 0.1.6

摘要见 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)，完整文本位于 [THIRD_PARTY_LICENSES/](THIRD_PARTY_LICENSES/)。

## 许可证

本项目使用 [GNU General Public License v3.0 or later](LICENSE)。

Copyright © 2026 Joey Kot <joey.kot.x@gmail.com>
