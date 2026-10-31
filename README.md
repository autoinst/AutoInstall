# AutoInstall

[![QQ-group](https://img.shields.io/badge/any_text-748826412-blue?logo=qq&label=QQ)](https://qm.qq.com/cgi-bin/qm/qr?k=eXjJGqiysJTIvxysfgODK--t7D9O25Wi&jump_from=webapi&authKey=UpmLLpiSPRzd2Tk3+jzfZ1oluywj53eNzz3j+o5pNut27ltqwcoRHCZb3iEfdCxD)
![gh-actions](https://img.shields.io/github/actions/workflow/status/autoinst/AutoInstall/go.yml)
![gh-lc](https://img.shields.io/github/last-commit/autoinst/AutoInstall)
![GitHub License](https://img.shields.io/github/license/autoinst/AutoInstall)
![GitHub Release](https://img.shields.io/github/v/release/autoinst/AutoInstall)


简单、快速和多功能的Minecraft服务端安装器  

AutoInstall使用镜像源[BMCLAPI](https://bmclapidoc.bangbang93.com/)与[MCIM API](https://github.com/mcmod-info-mirror)，得以在国内也能像各大启动器快速安装完成  
支持Forge,NeoForge,Fabric,Vanilla，或是使用CurseForge，Modrinth类型的整合包的安装  
感谢[DeEarthX](https://github.com/Tianpao/DeEarthX)为排除客户端Mod提供了参考

## 安装

> [!WARNING]
> **该应用还在开发，请先备份已有服务端文件，并尽量在新的空目录安装。**
> 若是在[简幻欢](https://simpfun.cn)中安装，可以在第三方镜像中搜索安装。支持开发者，可在[注册](https://simpfun.cn/auth?type=register&code=1332029)时使用 `1332029` 支持我，或是[爱发电](https://afdian.com/a/jdnjk)。

1. 从 [GitHub Release](https://github.com/autoinst/AutoInstall/releases) 下载对应系统和架构的程序并解压，准备所需 Java 环境。
2. 在服务端目录执行程序。可以直接提供命令行参数，也可以使用可选的 `inst.json`，或导入 CurseForge、Modrinth、ServerPackCreator 整合包。
3. 安装成功并校验产物后生成 `run.sh`。安装器不会自动启动 Minecraft；在 Linux 上可检查脚本后执行 `bash run.sh`。

以下命令以 Linux 的 `./AutoInstall` 为例，实际程序文件名以下载结果为准。

### 不使用配置文件

```bash
# 普通原版安装；--no-modpack 禁止扫描目录中的旧整合包
./AutoInstall --no-modpack --mc-version 1.21.1 --loader vanilla

# 普通 NeoForge 安装
./AutoInstall --no-modpack --mc-version 1.21.1 --loader neoforge --loader-version 21.1.123

# 整合包决定 Minecraft 和加载器版本，命令行只调整执行选项
./AutoInstall --modpack ./modpack.mrpack --download bmclapi --maxconnections 10 --maxretries 3

# 查看帮助或 AutoInstall 自身版本，不进行安装
./AutoInstall --help
./AutoInstall --version
```

### 安装目标与整合包选择

- **普通安装**：显式目标参数优先于 `inst.json` 的 `version`、`loader`、`loaderVersion`。必须明确 Minecraft 版本；加载器缺省为 `vanilla`。非原版加载器还需指定版本，可以使用 `latest`；原版不使用加载器版本。
- **整合包安装**：默认目标来自包内依赖声明，不使用 `inst.json` 中旧的 Minecraft/加载器/加载器版本。包自身的发布版本号不作为 Minecraft 版本。整合包安装不会创建、改写或删除用户 `inst.json`，也不会通过包内 overrides 覆盖它。
- **输入选择**：`--modpack <路径>` 只处理指定整合包或清单，失败不会回退到其他输入。支持归档 `.mrpack` / `.zip` 和清单 `modrinth.index.json` / `manifest.json` / `variables.txt`。
- 没有 `--modpack` 时，只要显式提供任意目标参数（`--mc-version`、`--loader`、`--loader-version`），就进入普通安装，不会被目录中的旧包抢占。`--no-modpack` 也可强制普通安装，不能与 `--modpack` 同用。
- 未提供目标参数且没有 `--no-modpack` 时，保留自动发现：优先查找包归档；多个归档可命名为 `modpack.mrpack` / `modpack.zip` 指定优先项，否则报歧义错误。无归档时尝试受支持的散装清单。损坏或不可识别的输入会报错，不作为“无包”忽略。
- 要**覆盖整合包目标**，必须同时显式指定 `--modpack` 和对应目标参数。切换到另一非原版加载器时还需明确 `--loader-version`（可用 `latest`），不能沿用原加载器的版本。

```bash
# 显式覆盖会输出原目标、新目标及兼容性警告
./AutoInstall --modpack ./modpack.mrpack --mc-version 1.21.1
```

覆盖只改变服务端安装目标，**不会转换或升级包内模组**，兼容性需自行确认。`latest` 保留各加载器现有选择规则，不统一为同一种“最新版”；请优先使用整合包声明或已确认兼容的固定版本。

### 可选的 inst.json

文件不是必需的。普通安装可使用以下示例；也可仅保留执行选项用于整合包安装。

```json
{
  "version": "1.21.1",
  "loader": "neoforge",
  "loaderVersion": "21.1.123",
  "download": "official",
  "maxconnections": 4,
  "maxretries": 3,
  "argsment": "",
  "followModpackFilenames": true,
  "enableModClassifier": true
}
```

执行选项的优先级是 **显式命令行参数 > 可选配置字段 > 默认值**，但整合包模式有一项例外：配置中的 `download` 不参与，下载源由 `--download` 或默认 `official` 决定。包模式只读取配置中的并发、重试、JVM 参数和两个开关；旧的目标字段不参与目标选择。配置文件仍必须是合法 JSON 对象。

| 配置字段 | 命令行参数 | 默认值 / 说明 |
| --- | --- | --- |
| `version` | `--mc-version` | 无默认 Minecraft 版本，缺少目标时报错 |
| `loader` | `--loader` | `vanilla`；支持 `vanilla` / `fabric` / `forge` / `neoforge` |
| `loaderVersion` | `--loader-version` | 非原版必须提供，支持 `latest` |
| `download` | `--download` | `official`；支持 `bmclapi`，旧值 `none` 等同 `official` |
| `maxconnections` | `--maxconnections` | `4`，必须为正整数 |
| `maxretries` | `--maxretries` | `3`，每个来源的最多尝试次数，包含首次尝试，必须为正整数 |
| `argsment` | `--argsment` | 空字符串；显式空值可清空配置中的 JVM 参数 |
| `followModpackFilenames` | `--follow-modpack-filenames` | `true` |
| `enableModClassifier` | `--mod-classifier` | `true` |

其他参数：`--clean` 在安装成功后清理本次拥有的临时内容；`--help` 显示帮助；`--version` 输出 AutoInstall 版本而非 Minecraft 版本。显式空目标/包路径以及非正并发或重试次数会报错。

### 模组命名与分类开关

两个开关都**默认开启**。JSON 中使用布尔值 `true` / `false`，不要写成字符串。命令行关闭时必须使用 `=false`，不能写成 `--mod-classifier false`。

```bash
./AutoInstall --modpack ./modpack.mrpack --follow-modpack-filenames=false --mod-classifier=false

# JVM 参数以 - 开头时推荐使用等号；显式空值清空配置
./AutoInstall --no-modpack --mc-version 1.21.1 --argsment="-Xms2G -Xmx4G"
./AutoInstall --no-modpack --mc-version 1.21.1 --argsment=""
```

- **跟随命名开启**：模组使用包内声明的路径。关闭时保留声明目录，仅将模组文件名改为实际传输并校验成功的原始候选 URL 中的文件名，不使用重定向目标或响应头改名。非模组资源、配置及 overrides 不改名；同目标不同内容会报冲突，不静默覆盖。CurseForge 继续使用其可获得的文件来源和校验能力。
- **分类器开启**：保留 JAR/Mixin/Modrinth 等客户端检测和移入 `.rubbish` 的处理。重复回收时相同内容复用，不同内容另取编号文件名，不覆盖历史文件。关闭后跳过这类额外检测、相关 API 请求及移动，但不保证模组能在服务端运行。Modrinth 明确声明 `env.server=unsupported` 的文件仍会在下载前排除，Forge/NeoForge 的服务端依赖筛选也始终执行。

包内下载文件先在 staging 校验，再依次应用 `overrides` 和 Modrinth 的 `server-overrides`；包内覆盖顺序与既有用户文件的冲突保护分开处理。最终内容与目标目录已有文件不同时仍报冲突。暂不支持的 Modrinth 加载器依赖（例如 Quilt）会明确报错，不静默降级为原版。

### 下载、安装产物与 Java

下载会重试并在需要时使用备用源；不同候选不混用未完成分片，有可用哈希时校验后才接受结果。Forge/NeoForge 继续使用官方安装器执行处理器，不将客户端生成产物误当成远程服务端依赖。安装器退出成功后仍检查启动参数和元数据声明的关键产物，缺失或校验失败不会作为安装成功。

Java 选择保留简幻欢和 mise 支持；简幻欢/mise 的安装器和启动脚本均按目标 Minecraft 版本选择 Java 8/16/17/21/25，不再为现代加载器强制使用 Java 8。其他环境在可用时使用 `JAVA_HOME/bin/java`，否则使用 `PATH` 中的 Java，自行安装的 Java 仍需与目标版本兼容。自有启动脚本只在产物校验通过后替换；官方安装器自身可能留下脚本，失败时不能仅凭目录内是否有 `run.sh` 判断安装是否成功。

## 代码边界

- `internal/app`：按安装会话编排发现、配置、包准备、加载器安装和清理；依赖可注入，离线测试不需要启动 Java。
- `internal/config` / `internal/model`：可选配置、CLI 显式参数、统一请求、文件任务和安装计划。
- `internal/download`：独立 HTTP 客户端、重试、候选与哈希、续传及取消；不依赖旧 `core` / `pkg` 全局状态。
- `internal/modpack` / `internal/workspace`：CF/MR/SPC 输入适配、规划、安全 staging、受保护路径与冲突部署。下载、CF Key、日志及分类回调按会话传入。
- `internal/runtime` / `internal/loader`：Java 环境和版本策略、纯 Maven 兼容逻辑；Forge/NeoForge 共用明确的安装后半段，Fabric 保持独立流程。

本次采用渐进迁移：`core` / `pkg` 保留旧加载器、元数据和兼容入口，不是一次性目录重命名。旧加载器执行仍要求安装根目录为当前工作目录，CLI 没有额外的输出目录参数；兼容层的 HTTP/日志仍按单会话使用，不支持在同一进程并发执行多个真实加载器安装。新增整合包适配不应重新引入写回 `inst.json` 或全局等待组。

## 构建与测试

Go 版本以 `go.mod` 为准。正常构建（包括依赖准备）需要相应工具链及依赖：

```bash
go test ./...
go vet ./...
go build ./...

# build.sh 使用包级构建并注入版本；不会把 API Key 编进二进制
# 安装 CurseForge 包时，在运行环境中设置 CF_API_KEY，不要提交密钥
BUILD_NAME=AutoInstall bash ./build.sh
```

依赖和对应 Go 工具链已在本机缓存时，可离线验证：

```bash
GOTOOLCHAIN=local GOPROXY=off go test ./...
GOTOOLCHAIN=local GOPROXY=off go vet ./...
GOTOOLCHAIN=local GOPROXY=off go build ./...
```

Race 检测需要支持的平台、CGO 和 C 编译器，可运行 `CGO_ENABLED=1 go test -race ./...`。CI 先在 Linux amd64 上执行测试、vet 和 race 检测，通过后才交叉构建现有 Linux amd64/386/arm64/arm 发布产物，注入版本但不嵌入 API Key。离线单元测试使用临时目录及本地假服务/命令执行器，不代表已完成所有版本的真实联网安装验收。

### GitHub Release 自动发布

`.github/workflows/go.yml` 保留推送到 `master` 时的测试及构建附件，同时在 GitHub Release **发布**（`published`）时执行发布流程；仅创建 tag 或保存草稿不会触发发布。

1. 创建并发布 Release，tag 使用 `v1.4.2`、`v1.4.2-14200` 或 `v1.1.5.2-11520` 这类格式：以 `v` 开头，三至四段数字，可附加 `-数字` 构建号。
2. 测试、vet 和 race 检测通过后构建四种 Linux 架构。Release tag 去掉 `v` 后完整注入 `main.gitversion`，例如 `v1.4.2-14200` 对应 `AutoInstall-1.4.2-14200`。
3. 所有架构构建成功后，将 `AutoInstall-linux-{amd64,386,arm64,arm}-v1` 和 `SHA256SUMS` 上传到该 Release。重跑会替换同名附件。
4. 发布说明为空或只有空白时，使用上一个可达 tag 以来的非 merge 提交标题补充；首次发布最多取最近 50 条。已有说明保持不变。

公开构建不读取或嵌入 `CF_API_KEY`，无需在仓库 Actions Secrets 配置该 Key。安装 CurseForge 包时，请在实际运行程序的环境中设置 `CF_API_KEY`（例如通过面板的受保护环境变量），程序按会话读取，显式空值可禁用旧私有构建内置的 Key。不要把包含密钥的私有构建上传到公开附件。

Release 上传使用自动提供的 `GITHUB_TOKEN`，只有发布 job 获得 `contents: write` 权限，无需另配个人 token。Go 项目不需要示例中的 Gradle、APK 签名或 Bugly 步骤。

普通推送和本地构建仍默认使用 `1.4.2-<git hash>`，也可通过 `BUILD_VERSION` 显式覆盖；发布流程不会为改版本而修改或提交源码。
