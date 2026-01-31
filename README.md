# STT 客户端

## 简介

这是一个面向 Windows 平台的桌面语音转写（STT）客户端。支持从麦克风录音、将音频转码（通过 ffmpeg）、将文件上传到指定的 ASR 接口并将返回的文本自动粘贴到当前光标位置。提供全局热键控制、临时文件管理与可选缓存保存。

## 主要特性

- 支持配置文件（JSON）和命令行参数，命令行参数优先级更高。
- 启动没有 config.json 时可生成默认 config.json。
- 使用 PortAudio（通过 Go 绑定）进行录音。
- 录音输出为临时文件名以 `RecordTemp_<uuid>` 为前缀，如不开启缓存，则会在下一次启动程序时自动清理临时文件。
- 使用 ffmpeg 将 WAV 转码为指定 codec/container（默认 opus/ogg）。
- 上传重试机制（可配置重试次数与基准延迟）。
- 支持通过 ExtraConfig 合并自定义字段到上传请求中，可以通过该选项注入任意参数。
- 自动将识别结果写入剪贴板并模拟 Ctrl+V 粘贴（使用 keybd_event）。
- 可选 Windows 通知（建议不要开启，会引入不必要的延迟）。
- 两种热键绑定方式：RegisterHotKey（注册全局热键）或 WH_KEYBOARD_LL 低级键盘钩子（HotKeyHook）。
- 可选将录音与响应保存到缓存目录（cache-dir 与 keep-cache）。

## 先决条件

- 操作系统：Windows
- ffmpeg 可执行文件在环境变量中

## 构建

### 在 Windows 上本地构建（动态链接 PortAudio DLL）

Windows（开发 / 动态链接）——快速上手

1. 确保系统安装 Go 工具链（建议 Go 1.17+/1.18+）。
2. 在 Windows 上安装或放置 PortAudio 的 DLL（或将对应的 .lib 放在链接器可见位置）。将 ffmpeg 可执行文件加入 PATH（验证：`ffmpeg -version`）。
3. 获取依赖（模块模式）:

   go get github.com/gordonklaus/portaudio github.com/atotto/clipboard github.com/gen2brain/beeep github.com/go-audio/wav github.com/google/uuid github.com/micmonay/keybd_event golang.org/x/net/http2

4. 构建（动态方式，适合开发与调试）:

```bash
   go build -o stt.exe
```

### 在 Linux 环境下交叉编译以生成 Windows 静态可执行文件（包含交叉编译 PortAudio 并静态链接）

Linux 下交叉编译为 Windows 静态可执行（示例）

在 Debian/Ubuntu 环境中，交叉编译 PortAudio 并静态链接以生成 stt.exe 的示例步骤。请根据发行版与交叉编译器路径调整。

1. 安装构建工具与交叉工具链

```bash
apt update
apt install -y build-essential autoconf automake libtool pkg-config wget tar mingw-w64
```

2. 下载并解压 PortAudio 稳定版（示例）

```bash
wget https://files.portaudio.com/archives/pa_stable_v190700_20210406.tgz
tar xzf pa_stable_v190700_20210406.tgz
cd portaudio
```

3. 交叉编译并安装 PortAudio 为 Windows 静态库（输出到当前目录的 install 路径下）

```bash
mkdir build-win && cd build-win
../configure --host=x86_64-w64-mingw32 --prefix=$(pwd)/install --disable-shared --enable-static CC=x86_64-w64-mingw32-gcc
make -j$(nproc)
make install
```

4. 设置环境变量以指向交叉编译安装产物并启用 cgo / 交叉链接

```bash
export PA_WIN_PREFIX=$(pwd)/install
export PKG_CONFIG_PATH="${PA_WIN_PREFIX}/lib/pkgconfig:${PKG_CONFIG_PATH:-}"
export CC=x86_64-w64-mingw32-gcc
export CGO_ENABLED=1
export GOOS=windows
export GOARCH=amd64
export CGO_CFLAGS="-I${PA_WIN_PREFIX}/include"
export CGO_LDFLAGS="-L${PA_WIN_PREFIX}/lib -lportaudio -lwinmm -lole32 -lws2_32"
export PKG_CONFIG_ALLOW_CROSS=1
```

5. 初始化项目依赖（如果尚未创建 go.mod）

```bash
go mod init stt
go mod tidy
```

6. 交叉静态构建 stt.exe（尝试让链接器进行静态链接）

```bash
PKG_CONFIG_ALLOW_CROSS=1 go build -v -ldflags '-extldflags "-static"' -o stt.exe
```

#### 注意与故障排除

- 交叉静态链接在不同系统和交叉编译器组合下差异较大。常见问题包括找不到静态 CRT、缺少系统库或符号未定义。若遇到链接错误，请查看 go build 输出并确认 PA_WIN_PREFIX 指向的 lib 中包含 libportaudio.a 以及所需的系统库静态版本。
- 若无法生成完全静态二进制，可先生成依赖 DLL 的动态二进制以便开发调试，然后再逐步尝试静态化链接。
- 在 Windows 平台直接编译（本机编译）通常更简单：在 Windows 上安装 PortAudio 的开发包 / DLL 并直接执行 `go build`。
- 静态链接 CRT 或使用 -static 可能导致某些系统调用或网络行为差异（例如 DNS、SSL 库依赖）。

---

## 配置文件说明

程序默认查找当前目录下的 `config.json`。如果不存在且没有提供任何命令行相关参数，程序会生成一个默认的 `config.json` 并退出。

主要配置字段（默认值见括号）：

- API_ENDPOINT (string) — ASR 上传端点 URL
- TOKEN (string) — 授权 token（Bearer）
- MODEL (string)
- LANGUAGE (string)
- PROMPT (string)
- TEXT_PATH (string) — 从返回 JSON 中抽取文本的路径，点分并支持索引（默认 "text"）
- ExtraConfig (string) — 字符串化 JSON，会解析为根级字段并覆盖基础字段
- Channels (int) — 录音通道数（1）
- SAMPLING_RATE (int) — 采样率 Hz（16000）
- SAMPLING_RATE_DEPTH (int) — 采样位深（16）
- BIT_RATE (int) — 音频比特率 kbps（128）
- CODECS (string) — 编码器（"opus"）
- CONTAINER (string) — 容器（"ogg"）
- REQUEST_TIMEOUT (int) — 请求超时（秒，30）
- MAX_RETRY (int) — 上传最大重试次数（3）
- RETRY_BASE_DELAY (float) — 重试间隔基准秒（0.5）
- ENABLE_HTTP2 (bool) — 是否启用 HTTP/2（true）
- VERIFY_SSL (bool) — 是否验证 SSL（true）
- HOTKEY_HOOK (bool) — 是否使用低级钩子（false）
- StartKey / PauseKey / CancelKey (string) — 热键字符串（默认 "alt+q", "alt+s", "esc"）
- CACHE_DIR (string) — 缓存目录路径（空则使用当前目录）
- KEEP_CACHE (bool) — 是否保存录音与响应（false）
- NOTIFICATION (bool) — 是否启用通知（true）
- FFMPEG_DEBUG, RECORD_DEBUG, HOTKEY_DEBUG, UPLOAD_DEBUG (bool) — 各类调试开关

## 命令行参数

命令行参数优先级高于配置文件，会覆盖配置文件中的对应设置。常见用法：

- -config <path>         指定配置文件
- -file <path>           上传已有音频文件（跳过录音）
- -api-endpoint <url>
- -token <token>
- -model <model>
- -language <lang>
- -prompt <text>
- -text-path <path>      自定义从返回 JSON 中抽取文本的路径
- -extra-config <json>   额外 JSON 字符串，会解析并合并到请求 payload（优先级高）
- -codecs/-container/-channels/-sampling-rate/-sampling-rate-depth/-bit-rate
- -request-timeout/-max-retry/-retry-base-delay
- -start-key/-pause-key/-cancel-key/-hotkeyhook
- -cache-dir/-keep-cache
- -notification
- -ffmpeg-debug/-record-debug/-hotkey-debug/-upload-debug

### 示例

1. 生成默认配置文件（当目录无 config.json 且没有传入参数时程序会自动生成）:
   stt.exe

2. 使用命令行参数直接上传已有音频:
   stt.exe -api-endpoint https://api.example/v1/transcribe -token sk-xxx -file sample.wav

3. 使用配置文件启动并通过热键控制录音:
   stt.exe -config config.json

## 其他说明

### TEXT_PATH 与 ExtraConfig 说明

- TEXT_PATH：用于从 ASR 的 JSON 响应中定位最终文本。支持类似 "results[0].alternatives[0].transcript" 或简单 "text"。如果配置了 TEXT_PATH 并且解析成功，则该值即为结果（即使空字符串也作为有效结果返回）。
- ExtraConfig：接受一个 JSON 字符串（需转义），解析后将根级字段合并到上传表单中，优先级高于程序内置字段，方便将任意自定义字段、数组传给服务（例如 language_hints）。

### 临时文件与缓存

- 录音阶段会在临时目录（若配置了 cache-dir 则使用该目录，否则为当前工作目录）创建 RecordTemp_<uuid>.wav 与 RecordTemp_<uuid>.<ext>（ext 基于 container）。
- 程序启动会清理当前临时目录下以 RecordTemp_ 开头的文件。
- 若启用 keep-cache 且提供了 cache-dir，程序会将录音与转码输出与对应 JSON 响应按时间戳重命名并保存到 cache-dir 中。

### 热键与权限

- 默认热键：开始/停止 alt+q，暂停/恢复 alt+s，取消 esc。
- 两种实现：
  - RegisterHotKey: 使用 Windows RegisterHotKey API（需要消息循环与注册权限）。
  - HotKeyHook（低级钩子）：使用 WH_KEYBOARD_LL 拦截并可独占按键事件（需要更高权限）。
- 注册热键或安装钩子时可能失败（例如权限、冲突或系统策略），程序将在错误时退出并打印提示。

### 调试与常见问题

- 无法初始化 PortAudio：确认 PortAudio 已安装并可被链接，或在 Windows 下确保 DLL 在 PATH 中或放在可执行文件同目录。
- ffmpeg 转码失败：确保 ffmpeg 在 PATH 中；可以启用 -ffmpeg-debug 打印执行命令与 stderr。
- 热键注册失败：尝试以管理员权限运行，或改用不同的热键组合；检查程序是否在被安全软件限制。
- 上传失败：检查 API_ENDPOINT、TOKEN 是否配置正确；启用 -upload-debug 查看请求/响应内容。
- 粘贴失败：确保目标应用接受 Ctrl+V，且在粘贴期间焦点在目标输入框；可以观察通知提示。

### 安全注意

- 若将 VERIFY_SSL 设为 false，会跳过 HTTPS 证书验证 —— 这在不受信任网络下存在安全风险，请谨慎使用。
- 上传和日志中可能包含敏感信息（例如 token 或识别结果），请妥善保管。
