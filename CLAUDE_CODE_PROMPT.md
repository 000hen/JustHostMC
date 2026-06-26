# Build Prompt — Minecraft Server Manager for Windows（工作名稱：`McManager`，可自行更名）

你要從零建構一個 Windows 桌面應用程式,用來建立、執行與管理多個 Minecraft 伺服器,並上架到 Microsoft Store。請**逐里程碑(milestone)增量開發**:每完成一個里程碑,確認它能 build、且通過該里程碑的驗收標準(acceptance criteria)後,再進行下一個。

---

## 0. 如何工作

1. 先檢查工具鏈是否齊全(.NET SDK 8+、Go 1.22+、`protoc` 或 `buf`、Windows App SDK / WinUI 3 範本)。缺什麼就先列出並停下來說明。
2. 第一步永遠是定義 `.proto` 契約並接好 C# 與 Go 兩邊的 code generation,確認雙邊能 build 並透過一個 Health RPC 連通,再往下做。
3. 標記為「需確認的決策(DECISION)」的項目,動工前先向使用者確認;其餘照本文件進行。
4. 所有程式碼識別字與註解一律用英文。本文件的中文僅為說明。

---

## 1. 產品摘要

一個 Windows 桌面 app,讓使用者建立、執行、管理多個 Minecraft 伺服器:
- 支援 server 類型:**vanilla**、**Forge / NeoForge(modded)**、**Paper / Bukkit / Spigot(plugin)**。
- 每個 server 互相隔離,並有資源(記憶體)上限。
- 一鍵備份與還原。
- 透過 **Microsoft Store** 散布。
- **Zero config = 不要求任何既有環境**:使用者安裝後即可使用,**不需要預先安裝 Java / Docker / WSL**;app 會在需要時**自動下載相依套件**(例如某 server 需要特定版本 OpenJDK 就按需下載並快取)。
- **i18n-ready**:UI 全面外部化字串、支援多語言。**base 語言為 English**(fallback 也是 English),繁體中文 `zh-Hant` 等為附加翻譯;新增語言只需加資源檔、不改程式碼(詳見 §14)。

---

## 2. 架構(固定)

```
WinUI (C#)  <-- gRPC (loopback) -->  engine (Go)  <-- IsolationBackend -->  per-server processes
```

- **Frontend**:C# / WinUI 3(Windows App SDK),MVVM(`CommunityToolkit.Mvvm`)。打包為 **MSIX、full-trust desktop app**(`runFullTrust`)。
- **Backend**:Go 寫的 daemon(本文件稱 `engine`),負責 provision 與 supervise 所有 server。
- **IPC**:gRPC。app 啟動時把**內建的 `engine` 執行檔當子行程啟動**,透過 loopback gRPC channel 連線;app 結束時關閉 engine。
- **Console** 用 gRPC 雙向 streaming RPC。
- `.proto` 契約是**唯一真相來源**,C# 與 Go stub 都由它產生。

### IPC 安全
engine 啟動時由 app 產生一組 random session token 傳入(環境變數或啟動參數),engine 只接受帶此 token 的 gRPC 呼叫,避免同機其他行程劫持。engine 綁 `127.0.0.1` 的隨機 port,把 port 回報給 app(stdout 第一行或固定檔案)。

---

## 3. Repo 結構(monorepo)

```
/proto                 # .proto files（source of truth）
/engine                # Go backend
  /cmd/engine          # main
  /internal/grpc       # gRPC server impl
  /internal/provider   # server-type downloader adapters
  /internal/isolation  # IsolationBackend implementations
  /internal/backup     # backup / restore
  /internal/store      # SQLite server registry
/app                   # C# WinUI frontend
  /Services            # DaemonClient（gRPC）, EngineHost（launch child process）
  /ViewModels
  /Views
/build                 # MSIX packaging, signing, scripts
README.md
```

> 註:OpenJDK 等執行階段相依**不放在 repo / 套件內**,由 `engine` 在執行階段按需下載並快取於 app data(見 §5)。

---

## 4. gRPC 契約(最先定義)

先寫 `/proto/mcmanager/v1/mcmanager.proto`,大致如下(可擴充,但服務切分照此):

```proto
syntax = "proto3";
package mcmanager.v1;
option go_package = "github.com/yourorg/mcmanager/engine/gen/mcmanager/v1;mcmanagerv1";
option csharp_namespace = "McManager.Grpc";

enum ServerType {
  SERVER_TYPE_UNSPECIFIED = 0;
  VANILLA  = 1;
  PAPER    = 2;
  SPIGOT   = 3;
  FORGE    = 4;
  NEOFORGE = 5;
  FABRIC   = 6;
}

enum ServerStatus {
  SERVER_STATUS_UNSPECIFIED = 0;
  STOPPED    = 1;
  INSTALLING = 2;
  STARTING   = 3;
  RUNNING    = 4;
  STOPPING   = 5;
  CRASHED    = 6;
}

message Server {
  string id = 1;
  string name = 2;
  ServerType type = 3;
  string mc_version = 4;
  int32 memory_mb = 5;
  int32 port = 6;
  ServerStatus status = 7;
}

message CreateServerRequest {
  string name = 1;
  ServerType type = 2;
  string mc_version = 3;
  int32 memory_mb = 4;
  int32 port = 5;          // 0 = auto-assign
}

// ── i18n / message strategy ──────────────────────────────────────────────
// Dynamic, display-only messages travel as a localization KEY in the form
// "namespace.method.type" (e.g. "install.progress.downloading_server"); the
// frontend resolves the key against its .resw resources and formats with args.
// Programmatic outcomes (errors, actions) stay strongly typed as enum/struct.
message LocalizedMessage {
  string key = 1;                 // "namespace.method.type"
  map<string, string> args = 2;   // values for placeholders in the localized string
}

// Errors are returned via gRPC status (see §4.1), with ErrorDetail packed into
// the status details. ErrorCode is the programmatic discriminator; the frontend
// maps it to a localized string. metadata carries context (e.g. {"port":"25565"}).
enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  PORT_IN_USE         = 1;
  VERSION_NOT_FOUND   = 2;
  JRE_DOWNLOAD_FAILED = 3;
  DOCKER_UNAVAILABLE  = 4;
  INSTALL_FAILED      = 5;
}
message ErrorDetail {
  ErrorCode code = 1;
  map<string, string> metadata = 2;
}

// Streamed continuously during Create so the frontend can render a live install
// view: a current-step label, a progress bar, and a scrolling detail/log box.
message InstallProgress {
  LocalizedMessage step = 1;     // current high-level step (localized by frontend); set when the step changes
  double fraction = 2;           // 0..1 overall progress; < 0 means indeterminate (show a marquee bar)
  string log_line = 3;           // raw stdout/stderr passthrough (download detail, installer output…); append verbatim to the detail box; NOT localized
}

message ServerId { string id = 1; }
message Empty {}
message ServerList { repeated Server servers = 1; }

// Console: bidirectional stream
message ConsoleInput { string server_id = 1; string command = 2; }
message ConsoleEvent { string server_id = 1; string line = 2; ServerStatus status = 3; }

message Backup { string id = 1; string server_id = 2; int64 size_bytes = 3; string created_at = 4; }
message BackupList { repeated Backup backups = 1; }
message CreateBackupRequest { string server_id = 1; bool safe_online = 2; }   // safe_online = pause saves, snapshot, resume
message RestoreBackupRequest { string server_id = 1; string backup_id = 2; }

service EngineService {
  rpc Health(Empty) returns (Empty);
  rpc ListVersions(VersionQuery) returns (VersionList);   // available MC versions per type
}
message VersionQuery { ServerType type = 1; }
message VersionList { repeated string versions = 1; }

service ServerService {
  rpc List(Empty) returns (ServerList);
  rpc Create(CreateServerRequest) returns (stream InstallProgress);  // last message implies done
  rpc Start(ServerId) returns (Empty);
  rpc Stop(ServerId) returns (Empty);
  rpc Delete(ServerId) returns (Empty);
}

service ConsoleService {
  rpc Attach(stream ConsoleInput) returns (stream ConsoleEvent);
}

service BackupService {
  rpc Create(CreateBackupRequest) returns (Backup);
  rpc List(ServerId) returns (BackupList);
  rpc Restore(RestoreBackupRequest) returns (Empty);
  rpc Delete(Backup) returns (Empty);
}
```

C# 用 `Grpc.Tools` 從 proto 產生 client;Go 用 `protoc-gen-go` + `protoc-gen-go-grpc`(或 `buf`)。

### 4.1 錯誤處理（proper gRPC status + detail）

錯誤一律用**標準 gRPC status** 回傳,**不要**塞進正常回應的欄位:
- 選對 **canonical status code**:`PORT_IN_USE` → `ALREADY_EXISTS`(或 `FAILED_PRECONDITION`)、`VERSION_NOT_FOUND` → `NOT_FOUND`、`DOCKER_UNAVAILABLE` → `FAILED_PRECONDITION` / `UNAVAILABLE`、`JRE_DOWNLOAD_FAILED` → `UNAVAILABLE`、`INSTALL_FAILED` → `INTERNAL`、參數錯誤 → `INVALID_ARGUMENT`。
- 在 status 的 **details** 附上 `ErrorDetail`(帶 `ErrorCode` 與 metadata),作為程式可判讀的判別依據。
- **Go**:`google.golang.org/grpc/status` + `google.golang.org/grpc/codes` 建 status,`st, _ := st.WithDetails(&ErrorDetail{...})` 後 `return nil, st.Err()`。(也可改用 `google.golang.org/genproto/googleapis/rpc/errdetails` 的標準 `ErrorInfo`,擇一即可,但全專案一致。)
- **C#**:catch `RpcException`,讀 `ex.StatusCode`,再用 rich status(`ex.GetRpcStatus()` → `Google.Rpc.Status.Details`,`TryUnpack<ErrorDetail>()`)取出 `ErrorCode`,對應為當地語言字串顯示。

---

## 5. Server 類型(downloader adapter 模式)

在 Go 定義一個 `Provider` 介面,每種類型一個實作。每個 provider 要能回報:下載來源、安裝步驟、**所需 Java major 版本**、以及最終啟動指令。

```go
type Provider interface {
    Versions(ctx context.Context) ([]string, error)
    // Install resolves & downloads files into dir, runs any installer,
    // and returns the launch spec (jar / args) plus required Java major.
    Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error)
}

type LaunchSpec struct {
    JavaMajor int      // 8 / 17 / 21 ...
    Args      []string // e.g. ["-jar", "server.jar", "nogui"]
}

// Progress is reported continuously during Install and maps onto InstallProgress.
// Emit Step when the high-level step changes, Fraction for known-size downloads
// (negative = indeterminate), and stream LogLine for EVERY line of raw output —
// download detail plus the piped stdout/stderr of installers (Forge installer,
// Spigot BuildTools). LogLine is passthrough; never localize it.
type Progress struct {
    Step     string  // localization key "namespace.method.type"
    Fraction float64 // 0..1, or < 0 for indeterminate
    LogLine  string  // raw stdout/stderr line; optional
}
```

各類型實作要點:
- **Vanilla**:抓 `https://piston-meta.mojang.com/mc/game/version_manifest_v2.json` → 找到該版本的 per-version JSON → 讀 `downloads.server.url` 下載 `server.jar`。
- **Paper**:打 PaperMC 的 downloads API(依 MC 版本解析最新 build),下載對應 jar。**把 Paper 設為 plugin server 的預設值**(它是 Bukkit/Spigot 的 drop-in)。
- **Bukkit / Spigot**:⚠️ **不可重新散布預先編譯好的 Spigot/CraftBukkit jar(授權限制)**。若使用者選 Spigot,必須在本機下載並執行 **BuildTools** 產生 jar(需要 Java + git 環境,且耗時)。實作上優先引導使用者改用 Paper;真要支援 Spigot 就走 BuildTools 流程並在 UI 標示需時較久。
- **Forge / NeoForge**:下載對應的 **installer jar**,以 `--installServer` 在 server 目錄執行產生 server 檔案,再依產生的 run script / jar 決定啟動指令。
- **Fabric / Quilt(可選)**:走 Fabric meta API。

> 執行任何 installer / BuildTools 時,把它的 stdout/stderr **逐行透過 `Progress.LogLine` 串出**,讓前端的 detail 文字框即時顯示;下載階段則回報 `Fraction`(已知大小時)與簡短 `LogLine`(例如目前下載的檔名 / 位元組數)。

**Java 版本對應(按需下載)**:不同 MC 版本需要不同 Java(例如 1.21 需 Java 21,舊版可能需 17 或 8)。**不內建 JRE**;`engine` 依 `LaunchSpec.JavaMajor`,若本機快取尚無對應版本,就透過 **Adoptium / Eclipse Temurin API** 按需下載對應 OpenJDK 並快取重用(同一 major 版本只下載一次、跨 server 共用)。下載 URL 形如 `https://api.adoptium.net/v3/binary/latest/{feature_version}/ga/windows/x64/jre/hotspot/normal/eclipse`(`feature_version` 例如 `17`、`21`);可用 `https://api.adoptium.net/v3/info/available_releases` 查可用版本,並驗證隨附的 `.sha256.txt` checksum。**執行 server 只需 JRE(`image_type=jre`);只有 Spigot 走 BuildTools 編譯時才需要 JDK**(改用 `image_type=jdk`)。

---

## 6. 隔離後端：執行階段依環境自動選擇

本 app **不打包、也不安裝 Docker**,而是在執行階段依環境挑選隔離後端:

- **偵測**:app 啟動(或首次建立 server)時,偵測本機是否已安裝且正在執行 Docker Desktop。
- **若偵測到 Docker**:詢問使用者是否要用 Docker 取得較強隔離(二選一:「使用 Docker」/「直接在本機執行」);**使用者同意才用**。用 Docker Go SDK,每 server 一容器(mem/cpu limit、volume、port、attach stdio)。
- **若無 Docker 或使用者拒絕**:使用 **on-machine 行程 + Windows Job Objects** 作為後備,並**明確通知使用者「server 將直接在這台電腦上執行(非容器隔離)」**——首次執行時提示一次,並在每個 server 的卡片/詳情標示目前執行模式。Job Object 設記憶體硬上限與 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`(用 `golang.org/x/sys/windows`),確保 stop / app 關閉時整棵行程被乾淨終止;JVM 端 `-Xmx` 對齊上限。
- **絕不**自行安裝 Docker、**絕不**啟用 WSL2 / Hyper-V 或變更任何 Windows 系統設定。app 為 full-trust packaged,可存取使用者既有 Docker daemon 的 named pipe。

定義 `IsolationBackend` 介面,兩個實作:`JobObjectBackend`(後備預設)與 `DockerBackend`(偵測到且使用者同意時),執行階段依上述邏輯擇一。共同職責:start / stop / send-to-stdin / stream-stdout / 狀態查詢 / crash 偵測(可選自動重啟)/ 重新接管既有實例。

```go
type IsolationBackend interface {
    Start(ctx context.Context, spec InstanceSpec) (Instance, error)
    Stop(ctx context.Context, id string, graceful bool) error
    Attach(ctx context.Context, id string) (Instance, error) // re-adopt running instance
    List(ctx context.Context) ([]Instance, error)
}
```

---

## 7. 備份

- 一個 server 的備份 = 把它的資料夾(world + 設定 + 已安裝的 jar/mod/plugin 清單)打包成單一封存檔(`.zip` 或 `tar.zst`)。
- 支援:**on-demand** 與 **scheduled**。
- **Safe online backup(`safe_online=true`)**:對執行中的 server 先送 console 指令 `save-off` 與 `save-all`,做快照,再送 `save-on`,讓世界在不停服的情況下保持一致。
- **Restore**:從封存檔重建 / 取代某 server 的資料夾(還原前若在執行則先停)。
- 備份**可攜**(內部不含絕對路徑),存放在使用者可見的位置(例如 `Documents\McManager\Backups`),並提供匯出。

---

## 8. Microsoft Store 合規（硬性約束）

- 打包成 **MSIX、full-trust packaged desktop app**(`runFullTrust`),才能啟動子行程與使用 Win32(Job Objects)。
- **下載 server 軟體 / OpenJDK 執行階段 / mod / plugin = 下載第三方程式碼**:這是 app 核心功能(也是 zero config 的運作方式),**允許**,但:
  - 一律走 **HTTPS**。
  - 遵循政策 **10.2.9**(下載內容不能耗時過長、安裝成功率不能過低)。
  - 在送審備註清楚說明此下載行為。
  - 不得成為「繞過 Store 的通用 app 平台」或下載與本 app 功能無關的可執行碼。
- 提供 **privacy policy URL**(app/engine 會向下載來源傳輸資料,需揭露);若有收集任何個資需符合政策第 10.5 節。
- **乾淨解除安裝**:提供 app 內「移除所有資料(server / 備份 / 記錄 / 已下載的 JRE 快取)」動作;確保不留孤兒行程(Job Object 已保證子行程被清除)。文件清楚說明資料存放位置。
- **不得未經明確同意變更 Windows 設定或啟用 Windows 功能**:app **只使用使用者既有的 Docker**(偵測 + 徵得同意才用),**絕不**自行安裝 Docker 或啟用 WSL2 / Hyper-V;on-machine 後備模式同樣不更動任何系統設定。
- **內建 engine 執行檔**(套件內含);OpenJDK 等執行階段相依**按需下載並快取於 app data**(不打包,避免套件過大)——首次需要某版 Java 時才下載。
- capability 宣告需與實際功能相符(只宣告真正需要的網路 / 檔案系統權限)。
- metadata / 內容分級保持乾淨。

---

## 9. Frontend 細節

- MVVM(`CommunityToolkit.Mvvm`):`ObservableCollection<string>` 綁 console、`Status` 綁狀態、Start/Stop 綁 `RelayCommand`。
- **Engine 生命週期**:`EngineHost` 在 app 啟動時啟動 engine 子行程、讀回 port、建立 gRPC channel;app 關閉時優雅關閉 engine。
- **Console**:消費 `ConsoleService.Attach` 的雙向 stream;**每個收到的事件都要用 `DispatcherQueue.TryEnqueue` marshal 回 UI 執行緒**,絕不可在 gRPC callback 執行緒直接動 UI。
- 頁面:server 列表 / dashboard、建立 server 精靈(type → version → memory → port)、console、檔案管理(基本)、備份、設定。
- **i18n**:所有 UI 字串放 `.resw`(`Strings/<lang>/Resources.resw`),XAML 用 `x:Uid` 自動解析、code-behind 用 `ResourceLoader`;日期/數字/檔案大小用使用者 `CultureInfo` 格式化。後端的 `LocalizedMessage.key` 與 `ErrorCode` 都由前端對應為當地語言字串;錯誤從 gRPC rich status 的 details 取出(見 §4.1、§14)。
- **安裝 / 下載進度檢視**:建立 server(尤其需要下載 jar/JRE 或跑 installer 時)時,消費 `ServerService.Create` 的 `InstallProgress` stream 並顯示三樣東西:(1)**目前步驟**標籤(`step` 經在地化,例如「正在下載 server.jar…」)、(2)**進度條**(`fraction`;`< 0` 時顯示不確定型 marquee)、(3)一個**可捲動、唯讀的 detail 文字框**,把 `log_line`(engine / installer 的原始 stdout/stderr)逐行附加進去並自動捲到底。每筆 stream 訊息一樣要用 `DispatcherQueue.TryEnqueue` marshal 回 UI 執行緒;`log_line` 為原樣 passthrough、不翻譯。

---

## 10. 已知地雷（請做對）

1. **UI 執行緒 marshaling**(見上)。
2. **engine 重啟要能重新接管執行中的 server**:用持久化的 registry + `IsolationBackend.List/Attach` 重建狀態,絕不可弄丟正在跑的行程。
3. **Java 版本必須對應 MC 版本**,依 `LaunchSpec.JavaMajor` 取用本機快取或按需下載對應 JRE(見 §5)。
4. **優雅停止**:先送 `stop`(或先 `save-all` 再 stop),等待 timeout,逾時才透過 Job Object 強制終止。
5. **server registry 持久化(SQLite)**,讓狀態跨重啟存活。
6. **建立 / 啟動時偵測 port 衝突**。
7. **透明告知執行模式**:採 on-machine(非容器)模式時,必須讓使用者清楚知道 server 直接跑在他的電腦上,不可默默進行。
8. **不可硬編碼使用者可見字串**:一律走資源系統;後端永不回傳要直接顯示的英文 prose——動態訊息用 `namespace.method.type` key(`LocalizedMessage`)、錯誤/動作用 enum/struct,由前端在地化。Minecraft server console 輸出是原樣 passthrough,不翻譯。
9. **緩衝區要有上限**:console 與安裝 detail 的 in-memory 行緩衝一律用**有上限的 ring buffer**(例如各保留最後 1000–2000 行,超過丟最舊),避免長時間 / 高頻輸出(如 BuildTools)吃光記憶體、拖慢 UI;單行過長也要截斷。後端 replay 給新訂閱者的歷史同樣設上限。

---

## 11. 里程碑（建構順序，逐一驗收）

- **M0 — 骨架**:repo 結構、工具鏈檢查、`.proto` + 雙語 codegen 接好;engine 與 app 能 build 並透過 `Health` RPC 連通(含 session token 驗證);**建立 i18n 資源基礎建設(`.resw` + `en-US` / `zh-Hant`),此後所有 UI 字串一律走資源系統**。
  - 驗收:`dotnet build` 與 `go build` 皆通過;app 啟動後能呼叫 engine `Health` 成功;切換系統語言能看到對應語言的範例字串。
- **M1 — Vanilla 生命週期 + 按需 JRE**:Vanilla provider + `JobObjectBackend` + 按需下載對應 OpenJDK(Adoptium API)並快取,能 create → start → stop 一個 vanilla server,狀態正確,記憶體上限生效。建立流程顯示目前步驟 + 進度條 + detail 文字框(下載 server.jar 與 JRE 的原始輸出)。首次執行顯示「直接在本機執行」提示。
  - 驗收:在**沒有預裝 Java** 的機器上,建立過程中前端即時顯示下載步驟、進度與 detail 輸出;可實際啟動一個 vanilla server 並在 console log 看到 `Done (...)!`,stop 後行程樹被清除。
- **M2 — 串流 console**:end-to-end(engine ↔ gRPC stream ↔ WinUI),能即時看 log 並送指令。
  - 驗收:UI 即時顯示 server 輸出;輸入 `say hi` 反映在 log。
- **M3 — 持久化與韌性**:SQLite registry + 重啟後重新接管 + crash 偵測(可選自動重啟)。
  - 驗收:engine 重啟後仍掌握執行中的 server;手動 kill server 後狀態轉為 `CRASHED`。
- **M4 — 多 server 類型**:Paper provider + Forge/NeoForge(installer 流程)+ Java 版本選擇;Spigot 走 BuildTools(或引導改用 Paper)。installer / BuildTools 的 stdout/stderr 即時串到建立進度的 detail 文字框。
  - 驗收:能各建立並啟動一個 Paper 與一個 Forge server;建立 Forge 時 installer 輸出即時顯示在 detail 框。
- **M5 — 備份與記錄保留**:safe online 快照 + scheduled + restore + 可攜封存;安裝 / console 記錄持久化(可設定)+ TTL 保留政策(保留天數 + 大小上限 + 立即清除)。
  - 驗收:對執行中的 server 做 safe backup 不停服、還原後世界一致;安裝失敗後能在記錄找到原因;超過保留期 / 大小上限的舊記錄會被自動清除。
- **M6 — 上架就緒**:MSIX 打包(內建 engine 執行檔)、capability 宣告、`Package.appxmanifest` 宣告支援語言、乾淨解除安裝/移除資料(含已下載的 JRE 快取)、privacy policy stub、Store 送審 checklist(說明按需下載行為)。
  - 驗收:產出可安裝的 MSIX;通過 Windows App Certification Kit(WACK)基本檢查;系統語言切到繁中時 UI 全在地化、無漏網英文字串。
- **M7 — Docker 後端(偵測 + 同意)**:`DockerBackend` + 啟動時偵測 Docker Desktop + 徵得使用者同意才啟用;未啟用時沿用 M1 的 on-machine 後備。
  - 驗收:在已安裝 Docker 的機器上,使用者同意後 server 改在容器內執行;拒絕則退回 on-machine 模式且有明確標示。

---

## 12. 技術 / 函式庫選擇

- **Go**:`grpc-go`(含 `google.golang.org/grpc/status`、`google.golang.org/grpc/codes`)、錯誤細節用 `google.golang.org/genproto/googleapis/rpc/errdetails`(或自訂 `ErrorDetail`)、std `net/http`(下載)、`modernc.org/sqlite`(CGo-free,利於跨編譯)或 `mattn/go-sqlite3`、`golang.org/x/sys/windows`(Job Objects);Docker 後端用 `github.com/docker/docker/client`。
- **C#**:WinUI 3(Windows App SDK)、`Grpc.Net.Client` + `Google.Protobuf` + `Grpc.Tools`、rich status 用 `Grpc.StatusProto`(`Google.Api.CommonProtos`)、`CommunityToolkit.Mvvm`。
- **Codegen**:`protoc` 或 `buf`。

---

## 13. 慣例 / 測試 / 交付

- 程式碼識別字與註解一律英文。
- **i18n 從第一行 UI 就遵守(不可事後補)**:任何新增的使用者可見字串都必須走資源檔,不得硬編碼。
- 測試:provider 的 URL 解析、backup/restore 邏輯做 unit test;一個會啟動微型 vanilla server 的 integration test。
- 機密 / 設定不入原始碼。
- 產出 `README.md`,含 build、run、package、上架 checklist 說明。
- 每個里程碑結束時,簡述「做了什麼、如何驗證、下一步」。

---

## 14. 國際化（i18n）

從專案一開始就 i18n-ready,**不可事後補**(retrofit 成本極高)。

**base / default 語言 = English（`en-US`）**:English 是 source-of-truth、也是 fallback——任何語言缺字串時都退回 English。繁體中文(`zh-Hant`)及其他語言(`zh-Hans`、`ja` …)為**附加翻譯**。架構要做到「新增語言 = 加一個資源檔」,不改任何程式碼;在 `Package.appxmanifest` 把 default language 設為 `en-US`。

**字串歸屬（關鍵架構決策）**:
- **只有 frontend 產生使用者可見文字。** backend(engine)**永不回傳要直接顯示的 prose**,而是用兩種方式表達:
  - **動態顯示訊息**(進度、通知等)→ 帶 `namespace.method.type` 的 **localization key**(`LocalizedMessage`,見 §4),frontend 查 `.resw` 並用 args 格式化。新增訊息只要加一個 key + 資源字串,不動 proto。
  - **錯誤 / 動作結果**(程式需據以分支者)→ 強型別 **enum / struct**(`ErrorCode` 等,透過 gRPC status details 回傳,見 §4.1),frontend 把 enum 對應為當地語言字串。
- 兩種方式英文都不寫死在協定或後端;後端無需知道使用者語系。
- 例外:Minecraft server 的 **console 輸出是原樣 passthrough**(server 自己吐的 log),不翻譯、也不應翻譯。

**Frontend（WinUI）做法**:
- 資源檔放 `Strings/<language>/Resources.resw`(例如 `Strings/en-US/Resources.resw`、`Strings/zh-Hant/Resources.resw`)。
- XAML 用 `x:Uid`,讓 Resource Management System 依使用者語言自動解析;code-behind 用 `Microsoft.Windows.ApplicationModel.Resources.ResourceLoader`。
- 在 `Package.appxmanifest` 宣告支援語言。
- **不要串接可翻譯片段**(避免 `"已建立 " + name` 這種寫法),改用帶參數的格式字串。

**locale-sensitive 格式化**:日期/時間、數字、檔案大小**一律在前端用使用者 `CultureInfo` 格式化**;後端只給原始值(時間 ISO 8601、大小 bytes)。協定裡 `Backup.created_at` / `Backup.size_bytes` 之所以是 raw 值就是為此。

**版面**:不可假設英文長度寫死寬度(德文常更長、中文常更短),留文字伸縮空間;若日後支援 RTL(阿拉伯/希伯來文)別寫死 `FlowDirection`,目前可不實作但別擋路;用 pseudolocalization 抓漏網的硬編碼字串與爆版。

**Store**:在 Partner Center 為每個支援語言在地化商店列表 metadata(與 app 內字串分開維護)。

---

## 15. 記錄與保留（logging & retention）

**in-memory(即時顯示)**:console 與安裝 detail 用有上限的 ring buffer(見 §10 第 9 點),只負責「現在看得到的內容」,不是長期儲存。

**持久化（存到磁碟，可設定)**:
- 由 **engine** 負責寫(它握有行程 I/O)。預設開啟、可在設定關閉(對應「if needed」)。
- 保存對象:
  - **安裝 / 操作記錄**(每次 create / 安裝一份)——最值得存,失敗時這常是唯一線索;否則隨視窗關閉就消失。
  - **server console 記錄**(每個 server 一份滾動記錄)。注意 Minecraft server 本身已在其資料夾寫 `logs/latest.log` 與壓縮輪替記錄;app 可直接沿用/呈現那些,或另存一份含 manager 事件的捕捉記錄,**擇一即可、別重複造成混淆**。
  - **engine 診斷記錄**(排查 app 本身問題用)。
- 落地於 app data 下、可被「移除所有資料」清掉的目錄;**檔案輪替**(單檔到大小上限就換檔),避免單檔無限長大。

**TTL / 保留政策**:
- 背景清理工作,依**保留天數**(預設例如 14 天、可設定)刪除過舊記錄;另設**總量上限**(例如每 server 或全體 N MB,超過刪最舊)作為雙重保險。
- 設定頁提供:開關、保留天數、大小上限、「立即清除記錄」。
- 清理在 engine 啟動時與每日各跑一次。

**存取**:同機本地 app,frontend(full-trust)可直接讀 engine 的記錄目錄檢視;若想保持 engine 為單一權威,可加一個極簡 `LogService`(`ListLogs` / `OpenLog`),非必要。

**隱私 / 合規**:記錄可能含 server 輸出內容,屬「移除所有資料」範圍(§8),且需在 privacy policy 揭露其存放與保留方式。
