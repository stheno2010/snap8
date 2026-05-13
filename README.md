# snap8

`snap8.exe` を実行すると、その瞬間に**最前面のウィンドウを 2 行 × 4 列（= 8 分割）のグリッドに整列して、すぐ終了する**だけの Windows アプリです。常駐しません。Go 製・依存ゼロの単一 exe（ランタイム不要）。

- 整列の対象は「Alt+Tab に出るウィンドウ」を **Z オーダー（最近使った順）** で並べた先頭 N 個（N = 行 × 列 = 8）。9 個目以降は触りません。
- グリッドの行・列、隙間、対象モニターなどは `snap8.json` で変更できます。
- 終了コード: `0` = 整列した / `1` = 整列対象のウィンドウが無かった。

## 必要なもの

- Windows 10 / 11（64-bit）
- ビルドする場合のみ: Go 1.22 以降。**実行に追加ランタイムは不要**（単一の静的 exe）。

## 使い方

`snap8.exe` を実行すると、最前面のウィンドウを `Rows×Cols`（既定 2x4 = 8 分割）に整列して即終了します。コンソール窓は出ません。引数はありません。

### 好きなキーで呼びたいとき

`snap8.exe` への**ショートカット (.lnk)** を作り、そのプロパティの「ショートカット キー」欄に好きな組み合わせを入れてください（Windows の機能。`Ctrl+Alt+<キー>` 形式になります）。
あるいは StreamDeck・ランチャ・タスクバーへのピン留めなどから `snap8.exe` を実行する形でも OK です。

## ビルド

```powershell
cd C:\ws\snap8
go build -ldflags "-s -w -H windowsgui" -o snap8.exe
```

- `-H windowsgui` … GUI サブシステムにしてコンソール窓を出さない
- `-s -w` … デバッグ情報を落としてバイナリを小さく

実行はビルドした `snap8.exe` をダブルクリック / コマンド / ショートカットから。`snap8.json` は exe と同じフォルダに置いてください（無くても既定値で動きます）。

### アイコン

exe のアイコン（エクスプローラ／タスクバー／Alt+Tab に出る図）はリポジトリ同梱の `rsrc_windows_amd64.syso` に入っていて、`go build` が自動でリンクします（外部ツール不要）。差し替えたいときは `gen_icon.go` の図形・色を編集して再生成してください:

```powershell
go generate ./...   # = go run gen_icon.go : snap8.ico と rsrc_windows_amd64.syso を作り直す
go build -ldflags "-s -w -H windowsgui" -o snap8.exe
```

`gen_icon.go` は標準ライブラリだけで `.ico` と COFF リソースオブジェクト（`.syso`）を生成します（ImageMagick や windres、追加モジュールは不要）。

## 設定（`snap8.json`）

exe と同じフォルダの `snap8.json` を実行のたびに読みます（無ければ内蔵の既定値。不正な値・壊れた JSON はその項目だけ既定値に戻します）。

| キー | 既定 | 意味 |
|---|---|---|
| `Rows` | `2` | グリッドの行数 |
| `Cols` | `4` | グリッドの列数（→ `Rows × Cols` 個のウィンドウを整列） |
| `Gap` | `0` | ウィンドウ間・画面端の余白(px)。 |
| `UseWorkArea` | `true` | `true` = タスクバーを避けた作業領域に収める / `false` = 画面全体を使う。 |
| `TargetMonitor` | `"Cursor"` | どのモニターに整列するか。`"Cursor"`（マウスのある画面）/ `"Primary"` / `"Foreground"`。 |
| `RestoreMinimized` | `true` | `true` のとき最小化中のウィンドウも復元して整列対象に含める。 |

## 仕組み（概要）

1. `SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2)` で座標を物理ピクセルに固定。
2. `GetTopWindow` → `GetWindow(GW_HWNDNEXT)` で全トップレベルウィンドウを Z オーダー順に走査し、
   可視・タイトルあり・ツールウィンドウでない・cloaked でない・シェル系でない、を満たすものを抽出（= Alt+Tab 相当）。
3. 対象モニターの矩形（`GetMonitorInfo` の作業領域 or 画面全体）を `Rows × Cols` のセルに等分。端数は累積丸めで吸収。
4. 各ウィンドウを `ShowWindow(SW_RESTORE)` で復元してから `SetWindowPos` でセルへ移動。
   見えない余白（`DWMWA_EXTENDED_FRAME_BOUNDS` と `GetWindowRect` の差）を補正し、見た目をピクセル単位でセルに合わせる。

すべて `syscall.NewLazyDLL` 経由の Win32 直叩きで、外部モジュール依存はありません（標準ライブラリのみ）。

## 既知の制限

- 管理者権限で動いているウィンドウ（昇格プロセス）は、非昇格の snap8 からは移動できず無視されます。必要なら snap8 を「管理者として実行」してください。
- 最小サイズが大きい／固定サイズのアプリは、セルにフィットしきれないことがあります。
- UWP / ストアアプリはフレーム境界補正でほぼ揃いますが、アプリ側の制約で多少ずれる場合があります。
- 64-bit Windows 専用（`GetWindowLongPtrW` を使用）。

## ファイル構成

| ファイル | 役割 |
|---|---|
| `go.mod` | Go モジュール定義（依存なし） |
| `main.go` | エントリ。設定読み込み・一度だけ整列して終了 |
| `win32.go` | Win32 API バインディング（`syscall.NewLazyDLL`）と構造体・ヘルパー |
| `arrange.go` | ウィンドウ列挙＋セル計算＋移動（コアロジック） |
| `snap8.json` | 既定設定ファイル |
| `gen_icon.go` | アイコン生成ツール（`//go:build ignore`。標準ライブラリのみで `snap8.ico` と `rsrc_windows_amd64.syso` を作る） |
| `rsrc_windows_amd64.syso` | exe に埋め込まれるアイコンリソース（`go build` が自動リンク） |
| `snap8.ico` | 同じアイコンの `.ico` ファイル（ショートカット等で使う用） |
