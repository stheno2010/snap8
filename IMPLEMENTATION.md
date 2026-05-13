# snap8 — 実装サマリー

Go 製のウィンドウ整列アプリ **snap8** を作成。

**仕様**: `snap8.exe` を実行 → 最前面のウィンドウを 2 行 × 4 列（= 8 分割）のグリッドに整列 → 即終了。常駐しない／ホットキーも持たない（ワンショット専用）。依存ゼロの単一静的 exe（実行に追加ランタイム不要）。

## 経緯

当初 C# / .NET 8 WinForms で実装 → ワンショット専用に削った段階で「Win32 直叩きばかりなら別言語が向く」となり、**Go に書き直し**（C# 版・ビルド成果物は削除済み）。

## ファイル構成

| ファイル | 役割 |
|---|---|
| `go.mod` | Go モジュール定義（標準ライブラリのみ。外部 require なし） |
| `main.go` | エントリ。`snap8.json` 読み込み、一度だけ整列して終了。終了コード 0=整列した / 1=対象なし。引数なし |
| `win32.go` | Win32 API バインディング（`syscall.NewLazyDLL` 経由）、`rect`/`point`/`monitorInfo` 等の構造体とヘルパー（UTF-16 文字列、POINT 値渡しのパック等） |
| `arrange.go` | Alt+Tab相当のウィンドウをZオーダーで列挙→対象モニターをRows×Colsに等分→先頭8個を各セルへ移動（DWM拡張フレーム境界で見た目を補正） |
| `snap8.json` | 既定設定（Rows / Cols / Gap / UseWorkArea / TargetMonitor / RestoreMinimized） |
| `README.md` / `.gitignore` | 説明書 / 無視設定 |

## ビルド

```powershell
cd C:\ws\snap8
go build -ldflags "-s -w -H windowsgui" -o snap8.exe
```

`-H windowsgui` でコンソール窓を抑止、`-s -w` でバイナリ縮小。出力 `snap8.exe` は数MB前後の静的バイナリ（依存DLLは Windows 同梱の user32 / dwmapi のみ）。

## 実装メモ

- DPI: `SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)` を main 冒頭で呼ぶ（マニフェスト不要）。
- ウィンドウ列挙: `GetTopWindow` + `GetWindow(GW_HWNDNEXT)` で Z オーダー。`IsWindowVisible` / `GetAncestor(GA_ROOTOWNER)` / タイトル長 / `WS_EX_TOOLWINDOW`(かつ `WS_EX_APPWINDOW` なし) / `DWMWA_CLOAKED` / シェル系クラス名で絞り込み。
- セル計算: `gap` を外周＋セル間に適用、`innerW = areaW - (cols+1)*gap`、各セル境界は `divRound(c*innerW, cols)` の累積で算出（整数演算・round half up）。合計が必ず画面寸法に一致。
- 移動: `IsZoomed`/`IsIconic` なら `ShowWindow(SW_RESTORE)` → `GetWindowRect` と `DwmGetWindowAttribute(DWMWA_EXTENDED_FRAME_BOUNDS)` の差で“見えない余白”を補正 → `SetWindowPos(SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED)`。
- `MonitorFromPoint` は `POINT` を値渡し → amd64 では 8 バイトを 1 レジスタに詰めて渡す（`packPoint`）。
- 64-bit 専用（`GetWindowLongPtrW`）。
- JSON は標準 `encoding/json`（コメント非対応のため `snap8.json` はコメントなしの素の JSON）。`*bool` フィールドで「未指定なら true」を表現。

## 確認したこと

- Go 1.26.3 を導入（`winget install GoLang.Go`）
- `go vet ./...` → 指摘なし、`go build -ldflags "-s -w -H windowsgui" -o snap8.exe` → 成功（約 2.2 MB の静的 exe）
- `snap8.exe` 実行 → ウィンドウが 2x4 に整列・即終了・残存プロセスなし・終了コード 0 ✓（実機で確認）

## 注意点（README にも記載）

- 昇格プロセスのウィンドウは非昇格の snap8 からは動かせない
- 最小サイズの大きいアプリはセルに収まりきらないことがある
- 64-bit Windows 専用
