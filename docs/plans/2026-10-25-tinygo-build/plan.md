# wasm ビルドの TinyGo 切り替え 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: fc07d0312960f111cf47ee9f16728935e35bc533
- 作業ブランチ: plan/2026-10-25-tinygo-build
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画書の作成担当: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 編成の例外理由: なし
- 実装担当: claude-opus-5-5 / high。ビルド手順・CI・スクリプトの変更が中心で Go 本体の変更はないが、TinyGo 固有の挙動差の検証と計測の判断を伴うため通常段
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ実装 claude-fable-5-1 / high、レビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-tinygo-build/ の plan.md と request.md を、実装担当が作業ブランチへの commit に含める（明示して staging）
- 公開範囲: 実装担当は作業ブランチへの commit まで。検収後に采配役が push・PR 作成を行い、CI 通過後のマージ、本番デプロイ（`npx wrangler deploy`）、デプロイ後に本番の CPU 時間を docs/architecture.md へ反映する小さな追加 PR の作成とマージまで、利用者が承認済み。外部への通信は GitHub Releases からの公式配布物の取得だけで、送信はしない

## 背景・目的

Workers 無料プランの CPU 時間の目安は 1 リクエスト 10ms だが、本番の CPU 時間は中央値 31ms（PR #7 後、17〜92ms）で、立ち上がり直後は 80〜92ms に達する。workers-go v0.36.0 はリクエストごとに新しい wasm インスタンスを作って main から実行するため、wasm の読み込み・初期化の重さが毎リクエストの CPU 時間に乗る。

事前確認では、同じソースを TinyGo でビルドすると wasm が 7.4MB → 1.8MB（gzip 2,001 KiB → 651 KiB）に減り、Node 上の計測で start が 8〜9ms → 約 4ms、webhook の handle が 36〜43ms → 24〜26ms に減った。WebAssembly.compile の CPU も 18ms → 5.5ms だった。この PR では wasm のビルドを TinyGo 0.43.0 に切り替え、同じ効果を本番に載せる。あわせて、開発者と CI が同じ手順で TinyGo と wasm-opt を入手できるようにし、計測の比較手順を整える。

## スコープ

- 対象:
  - `package.json` の `build` を TinyGo ビルド（`workers-assets-gen -mode=tinygo` + `tinygo build -target wasm -no-debug ./cmd/worker`）に切り替える。通常の Go ビルドは環境変数 `WASM_BUILD=go` で選べるようにし、`build:go` として残す（比較と退避用）
  - TinyGo 0.43.0 と Binaryen version_133（wasm-opt）を公式配布物から取得し SHA-256 を検証して展開する `scripts/install-tinygo.sh`、ツールの所在と版を確かめてビルドする `scripts/build.sh`、版と hash を 1 か所で持つ `scripts/toolchain.env`、導入スクリプトと `build.sh` の失敗時の振る舞いを確かめる `scripts/check-toolchain-errors.sh`
  - `.gitignore` にツールの展開先 `/.tools/` を追加
  - `.github/workflows/ci.yml`: ツールの導入（キャッシュ付き）、TinyGo ビルド、`tinygo test ./internal/...`、サイズ検査の閾値の引き下げ
  - `scripts/bench-cpu.mjs`: 比較元（Go ビルド）の wasm_exec.js を指定できる `--wasm-exec` オプション
  - `README.md`（必要なもの・ビルドと開発・CPU 時間の計測・制限事項）と `docs/architecture.md`（指標の表の wasm サイズ、ビルドの説明、今後の検討事項）の更新
  - 検証と計測の記録（実装担当の完了報告と、采配役が転記する PR 本文）
- 対象外:
  - Go のソース（`cmd/`、`internal/`、`tools/`）の変更。TinyGo 対応のためのコード変更が必要になった場合は止まって報告する
  - TinyGo のオプション調整（`-opt`、`-gc=leaking`、`-panic=trap`）。既定値で切り替え、調整は本番の計測後に別 PR で検討する
  - workers-go の更新、wrangler の更新、`compatibility_date` の変更
  - Homebrew tap の利用（利用者が見送り済み）
  - `docs/architecture/*.svg` の図の変更
  - 本番 CPU 時間の数値を `docs/architecture.md` に反映すること（デプロイ後の計測を受けて、小さな追加 PR で行う）

## 方針

### ビルド手順

- `npm run build` は `bash scripts/build.sh` を呼ぶ。`build.sh` は環境変数 `WASM_BUILD` を読み、未設定または `tinygo` なら TinyGo で、`go` なら通常の Go でビルドする。それ以外の値はエラーで終了する。
- ツールの置き場所は環境変数 `TOOLS_DIR`（既定はリポジトリ直下の `.tools`）で差し替えられる。`build.sh` と `install-tinygo.sh` は `$TOOLS_DIR/tinygo` と `$TOOLS_DIR/binaryen` を使う。`install-tinygo.sh` が読む設定ファイルは環境変数 `TOOLCHAIN_ENV`（既定は `scripts/toolchain.env`）で差し替えられる。
- TinyGo ビルド（`WASM_BUILD` 未設定または `tinygo`）は次を行う。
  1. `scripts/toolchain.env` を読み、`$TOOLS_DIR/tinygo/bin` と `$TOOLS_DIR/binaryen/bin` が存在すれば PATH の先頭に加える（存在しなければ PATH 上のものを使う）
  2. `tinygo version` が `toolchain.env` の版（0.43.0）と一致すること、`wasm-opt` が PATH で見つかることを確かめ、どちらかが欠ければ `scripts/install-tinygo.sh` の実行を促すメッセージで失敗する
  3. `go run github.com/syumai/workers-go/cmd/workers-assets-gen -mode=tinygo`
  4. `tinygo build -o ./build/app.wasm -target wasm -no-debug ./cmd/worker`
- Go ビルド（`WASM_BUILD=go`）は TinyGo / wasm-opt の確認をせず、従来のコマンド（`go run github.com/syumai/workers-go/cmd/workers-assets-gen -mode=go` + `GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o ./build/app.wasm ./cmd/worker`）を実行する。
- `-mode=tinygo` は既定値だが、Go モードと区別するため明示する。workers-go のテンプレートは `./...` をビルドするが、このリポジトリには `tools/` 配下に別の main があるため `./cmd/worker` を指定する。
- `wrangler.jsonc` の `build.command` は `npm run build` のまま変えない。`npx wrangler deploy` で手元の TinyGo ビルドが走る。wrangler は `build.command` を実行するときに環境変数を引き継ぐので、`WASM_BUILD=go npx wrangler deploy` なら Go ビルドでデプロイされる。
- `package.json` の `build:go` は `WASM_BUILD=go bash scripts/build.sh`（同等の書き方でよい）にする。CI では使わない。TinyGo に問題が出たときの退避先と、bench の比較元の作成に使う。

### ツールの入手

- `scripts/install-tinygo.sh` は、`uname` から linux-amd64 / darwin-arm64 を判定し、GitHub Releases の公式 tarball（下表のファイル名。TinyGo は `tinygo0.43.0.<os>-<arch>.tar.gz`、Binaryen は `binaryen-version_133-<arch>-<os>.tar.gz` で、Binaryen の arch / os は `x86_64-linux`、`arm64-macos` と表記される）を一時ディレクトリに取得し、`toolchain.env`（`TOOLCHAIN_ENV` で差し替え可）に固定した SHA-256 と一致してから `$TOOLS_DIR/tinygo` と `$TOOLS_DIR/binaryen`（既定は `.tools/tinygo` と `.tools/binaryen`）に展開する（`--strip-components=1`）。一致しなければ展開せず失敗する。未対応の OS / arch は対応表を示して失敗する。
- 同じ版が展開済みなら再取得しない（展開先に版を記した目印ファイルを置く）。
- `toolchain.env` に置く値: TinyGo 0.43.0、Binaryen version_133、次の表の 4 件の tarball の SHA-256（GitHub Releases の asset digest。Binaryen はリリースに付属の .sha256 とも一致）。

| 配布物 | ファイル名 | 取得元 | SHA-256 |
|---|---|---|---|
| TinyGo 0.43.0 linux-amd64 | tinygo0.43.0.linux-amd64.tar.gz | https://github.com/tinygo-org/tinygo/releases/download/v0.43.0/tinygo0.43.0.linux-amd64.tar.gz | ca9fc6d055e3889462dea4450a2e42a92242d88b5c362a7ec39bdb563413d78a |
| TinyGo 0.43.0 darwin-arm64 | tinygo0.43.0.darwin-arm64.tar.gz | https://github.com/tinygo-org/tinygo/releases/download/v0.43.0/tinygo0.43.0.darwin-arm64.tar.gz | f5e0161f212fb69ad4650a4809e3ca66856e184892e80058f0d47abc64151812 |
| Binaryen version_133 linux（x86_64） | binaryen-version_133-x86_64-linux.tar.gz | https://github.com/WebAssembly/binaryen/releases/download/version_133/binaryen-version_133-x86_64-linux.tar.gz | 2dc9c7813f5375db93d96ead4b78222fcc3e2677bbb832297af4797782a37489 |
| Binaryen version_133 macOS（arm64） | binaryen-version_133-arm64-macos.tar.gz | https://github.com/WebAssembly/binaryen/releases/download/version_133/binaryen-version_133-arm64-macos.tar.gz | ad66da82ac13f163e424b1643f16c6dfcccc98b5966296b43e52d3cab04f84a8 |

- 既定の展開先 `.tools/` はリポジトリ直下で `.gitignore` に入れる。リポジトリの外に置く方法（PATH に手で追加）も `build.sh` は受け付ける。
- `TOOLS_DIR` を差し替えられるのは、`.tools/` の実体を動かさずに異常系（hash 不一致、ツールの欠如）を確かめるため。`build.sh` は `$TOOLS_DIR` 配下を自動で PATH に足すので、呼び出し元の PATH から外すだけではツールの欠如を作れない。また展開済みなら再取得しないので、正常に導入した後の `TOOLS_DIR` で hash だけを変えても検証に到達しない。`TOOLCHAIN_ENV` を差し替えられるのも同じく、追跡ファイル `scripts/toolchain.env` を変えずに hash 不一致を確かめるため。

### 異常系の確認（`scripts/check-toolchain-errors.sh`）

- 追跡ファイルを書き換えずに異常系を確かめるスクリプトを追加し、commit する。次を順に行う。
  1. 一時ディレクトリを作り、`scripts/toolchain.env` をそこへコピーして、実行中の OS 向けの TinyGo の SHA-256 を 1 文字変える（追跡ファイルは変えない）
  2. 空の一時ディレクトリを `TOOLS_DIR`、変えたコピーを `TOOLCHAIN_ENV` にして `bash scripts/install-tinygo.sh` を実行し、非 0 で終わること、`TOOLS_DIR` に `tinygo/` と `binaryen/` が作られていないことを確かめる
  3. TinyGo と wasm-opt を含まない PATH（Go・Node・OS の標準コマンドの場所だけ）で、(a) 空の `TOOLS_DIR` で `bash scripts/build.sh` を実行して導入方法を示すメッセージで非 0 終了すること、(b) 既定の `.tools/tinygo` だけを指す（`binaryen/` がない）`TOOLS_DIR` を一時ディレクトリにシンボリックリンクで作り、wasm-opt の欠如を示すメッセージで非 0 終了することを確かめる。`.tools/` の実体は動かさない
  4. `WASM_BUILD=foo bash scripts/build.sh` が非 0 で終わることを確かめる
  5. すべて期待どおりなら 0、どれか一つでも外れたら何が外れたかを表示して非 0 で終わる。一時ディレクトリは終了時に消す
- hash 不一致の確認で配布物の取得まで進むため、CI では実行しない。手元の検収で、証拠記録用ラッパー経由で `bash scripts/check-toolchain-errors.sh` を実行し、終了コード 0・作業ツリーに差分なしの記録を採用する。

### CI

- 既存の step（gofmt、`go mod tidy -diff`、`go vet`、`go test -race ./...`、`GOOS=js GOARCH=wasm go vet ./...`）は残す。TinyGo には vet がないため、js/wasm の `go vet` は型検査として引き続き価値がある。
- 追加・変更する step:
  - `actions/cache` で `.tools` をキャッシュする。key は `tinygo-${{ runner.os }}-${{ hashFiles('scripts/toolchain.env') }}`。miss のときだけ `bash scripts/install-tinygo.sh` が実際に取得する（スクリプトは展開済みなら何もしないので、常に実行してよい）
  - `tinygo version` と `wasm-opt --version` を表示する
  - Build は `npm ci && npm run build` のまま。`test -f build/app.wasm`、`test -f build/worker.mjs` に加えて `test -f build/wasm_exec.js` を確かめる
  - `tinygo test ./internal/...` を追加する（ホスト実行、約 34 秒）
  - Script size は `bash scripts/check-size.sh 1024` にして、誤って Go ビルドになったとき（gzip 約 2,000 KiB）に失敗させる。スクリプト自体の既定値 2560 は変えない
  - 任意: TinyGo の標準ライブラリのビルドキャッシュ（ランナーのホームディレクトリ配下の `.cache/tinygo`）を key `tinygo-cache-${{ runner.os }}-<TinyGo 版>-${{ hashFiles('go.sum') }}` でキャッシュする
- `scripts/check-toolchain-errors.sh` は配布物を取得するため CI では実行しない

### 計測

- `scripts/bench-cpu.mjs` に `--wasm-exec <path>` を追加する。省略時は従来どおり `build/wasm_exec.js`、次に Go の `GOROOT/lib/wasm/wasm_exec.js` を探す。Go ビルドの wasm は Go 用の wasm_exec.js を、TinyGo ビルドの wasm は TinyGo 用を必要とし、取り違えるとインポートの不一致で失敗するため、比較元は `app.wasm` と `wasm_exec.js` を一緒に退避する。
- 比較手順（この PR の計測）:
  1. ベース（main、fc07d03）の worktree で既存の `npm run build`（ベースでは通常の Go ビルド。ベースの `package.json` に `build:go` はない）を実行し、`build/app.wasm` と `build/wasm_exec.js` をリポジトリの外へ退避する。作業ブランチの `npm run build:go` は、受け入れ基準「`npm run build:go` が従来どおり成功する」で別に確かめる
  2. 作業ブランチで `npm run build`
  3. ベースと新ビルドを交互に 3 組、各 60 回で計測する（ベースは `--wasm-exec` で退避した wasm_exec.js を指定）
  4. 各組の中央値（start / handle / total）を表にして完了報告に記録し、采配役が PR 本文に転記する

### ドキュメント

- README: 「必要なもの」に TinyGo 0.43.0 と Binaryen（wasm-opt）を加え、`scripts/install-tinygo.sh` の使い方を書く。「ビルドと開発」の `npm run build` の説明を TinyGo に書き換え、`build:go` と `tinygo test ./internal/...` を追記する。`bash scripts/check-toolchain-errors.sh` で導入スクリプトと `build.sh` の失敗時の振る舞いを確かめられることを 1 行書く。「CPU 時間の計測」に `--wasm-exec` と比較元の退避の説明を加える。マージ後の利用者向けなので、比較手順は「比較したいビルド（例: `npm run build:go` の結果）の `app.wasm` と `wasm_exec.js` を退避し、`--wasm-exec` で対応する wasm_exec.js を指定する」という一般的な書き方にする（特定のベース SHA は書かない）。Go 版に戻してデプロイする手順として `WASM_BUILD=go npx wrangler deploy` を書く。「制限事項と設計上の決まり」の CPU 時間の項に TinyGo を使っている旨と、TinyGo 固有の注意（reflect・標準ライブラリの差は `tinygo test` で確かめる）を 1〜2 文で加える。
- docs/architecture.md: 指標の表の「wasm サイズ（gzip 後）」を本 PR の `check-size.sh` の値に更新し、注記の日付を更新する。「CPU 時間（本番）」はデプロイ後の計測まで据え置き、その旨を注記する。「ビルドとデプロイの流れ」の文に TinyGo でビルドする旨を加える。「今後の検討事項」の TinyGo の行を「採用済み。本番の CPU 時間はデプロイ後に更新」に書き換える。

### 代替案と選ばなかった理由

| 代替案 | 選ばなかった理由 |
|---|---|
| Homebrew の tap（tinygo-org/tools）で TinyGo を入れる | 利用者が見送り済み。版の固定と hash 検証が公式 tarball の方が明確 |
| CI では `.deb`（tinygo_0.43.0_amd64.deb）を使う | 手元と CI で入手経路が分かれ、版と hash の管理が 2 か所になる。`.deb` の SHA-256 も手元にない |
| サードパーティの GitHub Action（setup-tinygo 系）で導入する | 依存先が増え、wasm-opt は別途必要。公式 tarball + hash 検証の方が追跡しやすい |
| `package.json` の `build` にコマンドを直書きし、ラッパースクリプトを作らない | TinyGo や wasm-opt が無いときの失敗が分かりにくく、`.tools/` を PATH に通す手段がない。`wrangler deploy` が手元でビルドする運用では、版の確認を自動化したい |
| 通常の Go ビルドを完全に削除する | bench の比較元の作成と、TinyGo で問題が出たときの退避先として安価に残せる |
| `build:go` の後に `wrangler deploy --no-build` 等で build.command を飛ばす | wrangler の版によって扱いが違い、手順が分かりにくい。環境変数で build.sh の分岐を選ぶ方が `wrangler deploy` の通常の流れに乗る |
| `tinygo test -target wasm ./internal/...` を CI に入れる | 実行に Node が要り、テストがファイルを読む（countries.json）ため wasm 上で動くか未確認。ホスト実行で reflect・encoding/json の挙動差は検査できる。確認できれば後続で追加 |
| `-opt=2` や `-gc=leaking` で CPU をさらに削る | 効果と副作用（サイズ、メモリ）の計測が別に要る。切り替え単体の効果を本番で測ってから判断する |

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | `scripts/toolchain.env`（版と SHA-256）と `scripts/install-tinygo.sh`（OS/arch 判定、`TOOLCHAIN_ENV`（既定 `scripts/toolchain.env`）の読み込み、取得、hash 検証、`$TOOLS_DIR`（既定 `.tools/`）へ展開、冪等）を書き、`.gitignore` に `/.tools/` を追加する。手元（macOS arm64）で実行し、`tinygo version` と `wasm-opt --version` を確かめる。2 回目の実行（既定の `TOOLS_DIR`）が再取得せずに 0 で終了することを確かめる | - |
| 2 | `scripts/build.sh`（`WASM_BUILD` による分岐、`TOOLS_DIR` の PATH 設定、版の確認、assets-gen、tinygo build / go build）を書き、`package.json` の `build` を差し替え、`build:go`（`WASM_BUILD=go bash scripts/build.sh`）を追加する。`npm run build` が通り、`build/` の 3 ファイルがそろうことを確かめる。続けて `scripts/check-toolchain-errors.sh`（上記「異常系の確認」の 1〜5）を書き、`bash scripts/check-toolchain-errors.sh` が 0 で終わり、実行後に作業ツリーに差分がないことを確かめる | 1 |
| 3 | `tinygo test ./internal/...` を手元で実行し、4 パッケージが ok になることを確かめる | 1 |
| 4 | `.github/workflows/ci.yml` を更新する（cache + install、version 表示、build、wasm_exec.js の存在確認、tinygo test、check-size 1024） | 2, 3 |
| 5 | `scripts/bench-cpu.mjs` に `--wasm-exec` を追加する（引数解釈と `findWasmExec` の候補の先頭に入れる。usage とファイル冒頭の説明も更新） | - |
| 6 | 計測: ベース（fc07d03）の worktree で既存の `npm run build`（Go ビルド）の結果を退避し、交互 3 組 × 60 回で比較し、表にまとめる | 2, 5 |
| 7 | `wrangler dev` を起動し `scripts/local-smoke.mjs` を 3 回実行して全場面合格と、場面 4 のクイズの正解が回ごとに変わることを確かめる。ログの出方（イベント種別・返信の種類・ステータス）が従来と同じことを確かめる | 2 |
| 8 | `bash scripts/check-size.sh 1024` でサイズを記録する。`build/app.wasm` にビルド機のホームディレクトリなど絶対パスが残っていないことを `strings` で確かめる。`npm run build:go` が成功することを確かめる。`WASM_BUILD=go npx wrangler deploy --dry-run --outdir=dist` と `WASM_BUILD` 未設定の `npx wrangler deploy --dry-run --outdir=dist` で表示される gzip サイズを記録する（最後は未設定の方を実行して build/ を TinyGo 版にしておく） | 2 |
| 9 | README と docs/architecture.md を更新する（上記「ドキュメント」の範囲） | 6, 8 |
| 10 | privacy 検査（個人のパス・ユーザー名が差分と計画書に無いこと）、`gofmt`・`go vet`・`go test -race ./...` の再確認、計画書（plan.md、request.md）の配置、作業ブランチへの commit | 4, 7, 9 |

## 完了条件・受け入れ基準

ビルドとツール
- [ ] `bash scripts/install-tinygo.sh` が macOS arm64 と Linux amd64 で成功する。2 回目の実行（既定の `TOOLS_DIR`）は再取得せずに 0 で終了する
- [ ] `npm run build` が TinyGo でビルドし、`build/app.wasm`、`build/worker.mjs`、`build/wasm_exec.js` が作られる。`build/wasm_exec.js` は TinyGo 用である（workers-go の `assets/wasm_exec_tinygo.js` と同じ内容）
- [ ] 異常系: `bash scripts/check-toolchain-errors.sh` が 0 で終わる。中身は次のとおりで、追跡ファイルは書き換えない。(1) hash 不一致: 空の一時ディレクトリを `TOOLS_DIR`、実行中の OS 向けの TinyGo の SHA-256 を 1 文字変えた `scripts/toolchain.env` の一時コピーを `TOOLCHAIN_ENV` にして `bash scripts/install-tinygo.sh` を実行すると、非 0 で終了し、`TOOLS_DIR` に `tinygo/` と `binaryen/` が作られていない。(2) ツール欠如: TinyGo と wasm-opt を含まない PATH（Go・Node・OS の標準コマンドの場所だけ）で、(a) 空の `TOOLS_DIR` で `bash scripts/build.sh` を実行すると導入方法を示すメッセージで非 0 終了する。(b) 既定の `.tools/tinygo` だけを指す（`binaryen/` がない）`TOOLS_DIR` を一時ディレクトリにシンボリックリンクで作って実行すると、wasm-opt の欠如を示すメッセージで非 0 終了する。`.tools/` の実体は動かさない。(3) `WASM_BUILD=foo bash scripts/build.sh` が非 0 で終わる。どれか一つでも外れたら何が外れたかを表示して非 0 で終わり、一時ディレクトリは終了時に消す。CI では実行せず、手元で証拠記録用ラッパー経由で実行した、終了コード 0・作業ツリーに差分なしの記録を採用する
- [ ] `npm run build:go` が従来どおり成功する
- [ ] `WASM_BUILD=go npx wrangler deploy --dry-run --outdir=dist` が成功し、表示される gzip サイズが Go ビルドの大きさ（約 2,000 KiB）になる。`WASM_BUILD` 未設定の `npx wrangler deploy --dry-run --outdir=dist` は TinyGo ビルド（1,024 KiB 以下）になる
- [ ] `build/app.wasm` が 2.5MB 未満（事前確認では 1.8MB）。`strings build/app.wasm` にビルド機のホームディレクトリの文字列が含まれない
- [ ] `bash scripts/check-size.sh 1024` が合格する（事前確認では gzip 651 KiB）

テストと検査
- [ ] `gofmt -l .` が空、`go mod tidy -diff` が差分なし、`go vet ./...` と `GOOS=js GOARCH=wasm go vet ./...` が成功、`go test -race ./...` が成功
- [ ] `tinygo test ./internal/...` で `internal/app`、`internal/bot`、`internal/country`、`internal/line` の 4 パッケージが ok
- [ ] CI（GitHub Actions、ubuntu-latest）が PR で全 step 成功する。2 回目以降の実行で `.tools` のキャッシュが hit する（采配役が push 後に Actions のログで確認）

計測（Node、`scripts/bench-cpu.mjs`、ベース = main fc07d03 の Go ビルド）
- [ ] ベースと新ビルドを交互に 3 組、各 60 回で計測し、組ごとの中央値（start / handle / total、3 場面）を表にして完了報告に記録し、采配役が PR 本文に転記する
- [ ] 3 組すべてで、3 場面の start と total が新ビルドの方が小さい
- [ ] start は 3 組の中央値の中央値でベースの 70% 以下（事前確認では 8〜9ms → 約 4ms）
- [ ] webhook 日本・webhook クイズの handle は 3 組すべてで新ビルドの方が小さい。GET / の handle は増えてもよいが total が減ること（事前確認では handle 2.54 → 3.97、total 11.01 → 7.74）
- [ ] 計測のぶれは数 ms あるため、1 組だけ逆転した場合は同じ条件でもう 1 組追加し、4 組中 3 組以上で上の向きなら合格とする

疎通（`wrangler dev --ip 127.0.0.1 --port 8787` + `node scripts/local-smoke.mjs`）
- [ ] `SMOKE TEST PASSED`。全場面が ok で、返信する場面の `elapsedMs` が `REPLY_DELAY_MS` 以上 2000 未満
- [ ] 3 回実行して、場面 4（クイズ）の `result=1` の選択肢が少なくとも 2 回異なる（インスタンスごとに乱数の種が変わること）
- [ ] wrangler dev のコンソールに、イベント種別・返信の種類・国コード・ローディング表示の結果・返信 API のステータスのログが従来と同じ項目で出る。発話本文やトークンが出ていない

ドキュメントと衛生
- [ ] README の「必要なもの」「ビルドと開発」「CPU 時間の計測」「制限事項と設計上の決まり」が TinyGo の手順と一致している。docs/architecture.md の wasm サイズと日付、ビルドの説明、今後の検討事項が更新されている
- [ ] 差分と計画書に個人環境のパス・ユーザー名・私的なリンクが無い。`.tools/` と `build/` が未追跡

デプロイ後（利用者が実施、合格ラインは目安）
- [ ] 手元で `npx wrangler deploy` が TinyGo ビルドで成功し、wrangler の表示する gzip サイズが CI と同程度（1,024 KiB 以下）
- [ ] LINE 実機で、国名ヒット（日本、イギリス）、国名ミス、クイズ、正解の回答、非テキストの 5 場面が従来と同じ返信になる
- [ ] `npx wrangler tail` で 10 件以上の CPU 時間と応答時間を記録し、CPU 時間の中央値が PR #7 後の 31ms より小さい。立ち上がり直後を除く最大値が従来の最大 92ms より小さい。応答時間（約 1.2 秒）が変わらない。10ms は目標であって合格ラインにしない
- [ ] 記録した値で docs/architecture.md の「CPU 時間（本番）」を更新する（デプロイ後の小さな追加 PR。利用者が承認済み）

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| 手元への TinyGo / wasm-opt の導入方法 | 確定: `scripts/install-tinygo.sh` で公式 tarball を hash 検証して `.tools/` に展開する（CI と同一手順、版が 1 か所）。手動で展開して PATH に追加する方法や Homebrew tap は採らない | この方式で実装する。`build.sh` は PATH 上の同じ版も受け付ける |
| macOS arm64 での TinyGo 動作 | 事前確認は開発者の手元と同じ macOS arm64 で、公式の darwin-arm64 版 TinyGo 0.43.0 と Binaryen version_133 の wasm-opt を使って `tinygo build -target wasm` と `tinygo test ./internal/...` が通っている。本番デプロイはこの環境でビルドする | タスク 1〜2 を最初に手元で実施する。`.tools/` に展開した版で `tinygo build` が失敗したら、エラー全文と `tinygo version`、`wasm-opt --version` を添えて止まって報告する |
| 通常の Go ビルドを残すか | 確定: `build.sh` の `WASM_BUILD=go` の分岐として残し、`build:go` から呼ぶ。TinyGo で問題が出たときの戻し先（`WASM_BUILD=go npx wrangler deploy`）と、比較元の作成に使う。CI では使わない | `build:go` を残す。CI に Go ビルドの step を追加しない（js/wasm の go vet は残す） |
| CI でのツール入手とキャッシュ | 同一スクリプト + actions/cache（推奨）/ `.deb` / サードパーティ Action。`.tools` の展開後サイズは数百 MB の見込み | 推奨案で実装する。cache の保存に失敗する、または展開後サイズが 1GB を超える場合は tarball だけをキャッシュする方式に変える（任意判断） |
| `tinygo test` を CI に入れる範囲 | `./internal/...` のホスト実行（推奨、約 34 秒）。`-target wasm` は未確認 | `./internal/...` を入れる。`-target wasm` は試してよいが、失敗や 2 分超なら入れず、結果を報告に書く。`tools/` と `cmd/worker` は対象外 |
| サイズ検査の閾値 | CI で `check-size.sh 1024` にして Go ビルドへの退行を検知する（推奨）/ 既定の 2560 のまま | 1024 で実装する。スクリプトの既定値は変えない |
| bench-cpu の wasm_exec.js の探し方 | `--wasm-exec` オプションの追加（推奨、最小）/ wasm のインポート（`wasi_snapshot_preview1` の有無）から自動判定 | オプションを実装する。自動判定は実装が 20 行程度で収まり、両方の wasm で動作確認できるなら追加してよい |
| README の書き換え範囲 | 4 節の更新（推奨）/ 全面改訂 | 上記 4 節と architecture.md の該当箇所に限る。構成図（svg）は触らない |
| TinyGo の最適化オプション | `-opt`（既定 z）、`-gc`、`-panic` は既定のまま | 変えない。計測で気づいた点は報告に書き、別 PR で検討する |
| 乱数の種（クイズ） | クイズは `math/rand/v2` の乱数をリクエストごとに作る。事前確認の TinyGo ビルドの計測では、インスタンスごとに出題国が変わっていた（例: TH、ZW、ME）。種が固定になると毎回同じクイズになる | タスク 7 で再確認する。3 回とも同じ正解なら止まって報告する（Go のソース変更が必要になる可能性があるため） |
| reflect / encoding/json の Unmarshal | Webhook の解析は `encoding/json` の Unmarshal（reflect）を使う。TinyGo 0.4x では構造体への Unmarshal に対応するが、埋め込み・`interface{}`・`json.RawMessage` などで差が出る可能性がある | `tinygo test ./internal/line` と local-smoke の 8 場面で確かめる。差が見つかればテストで再現し止まって報告する |
| goroutine と `time.Sleep`（asyncify） | ローディング表示後の待機と fetch は asyncify スケジューラで動く。事前確認では local-smoke の rhythm 検査（ローディング → 待機 → 返信、2 秒未満）に合格している | タスク 7 で再確認する。`elapsedMs` が待機時間を下回る、または 2000 を超えるなら止まって報告する |
| panic 時の挙動 | TinyGo はパニック時に Go のような goroutine のスタックトレースを出さない。`-no-debug` では位置情報も薄い。本番で原因調査がしづらくなる | 挙動の変更はしない。README の制限事項に 1 文で書く |
| GC | TinyGo の GC はヒープが埋まったときに走る。1 インスタンスが 1 リクエストしか処理しないため影響は小さい見込みだが、Node 計測の handle に含まれる | 計測で確認し、特別な対処はしない |
| 標準ライブラリの未対応部分 | `crypto/hmac`、`crypto/sha256`、`encoding/base64`、`net/url`、`log`、`fmt` は対応済み。ビルドは事前確認で通っている | ビルドが通ることを確認基準とする。警告（unsupported など）が出たら報告に記録する |
| ログの出方 | TinyGo の `log` は wasm_exec の fd_write 経由で出る。改行や前置きが変わると `wrangler tail` の読み取りに影響する | タスク 7 で従来のログと同じ項目が出ることを確かめる。差があれば報告し、ソース変更はしない |
| ビルド時間 | TinyGo は初回に標準ライブラリをビルドし約 14 秒。`wrangler deploy` が手元でビルドするため、デプロイの所要時間が延びる | README に書くだけ。CI では TinyGo のキャッシュで短縮を試す（任意） |
| 本番の CPU 時間の目標 | 10ms は目安。Node 計測では handle の半分以上が fetch 側の CPU で、ビルドでは減らない | 合格ラインは「中央値の減少」とし、10ms 到達を条件にしない |
| 本番の値の docs への反映 | 確定: デプロイと実機確認の後、本番の CPU 時間と応答時間を docs/architecture.md の指標の表に反映する小さな追加 PR を作る | 采配役が作成し、CI 通過後にマージする（利用者が承認済み）。この PR の実装担当の作業には含めない |
| 他の開発者向け文書の有無 | AGENTS.md / CLAUDE.md / CONTRIBUTING などにビルド手順が書かれていれば更新対象 | 存在すれば同じ内容に更新し、報告に書く |

## 公開経路

1. 実装担当が作業ブランチ plan/2026-10-25-tinygo-build に commit する（push はしない）
2. 検収後、采配役が push し、main への PR を作成する。CI（test job）が通ること、`.tools` のキャッシュが 2 回目で hit することを確認する
3. 実装レビューの合格後にマージする
4. 采配役が手元で `bash scripts/install-tinygo.sh` を実行し、`npx wrangler deploy` で本番に反映する（利用者が承認済み）。デプロイは手元で `npm run build` を走らせるため、TinyGo が手元で動くことが前提
5. 利用者が LINE 実機で確認し、`npx wrangler tail` で CPU 時間と応答時間を記録する。問題があれば `WASM_BUILD=go npx wrangler deploy` で再デプロイすれば従来の状態（Go ビルド）に戻せる（wrangler が `build.command` の `npm run build` に環境変数を引き継ぎ、`build.sh` が Go ビルドを選ぶ）
6. 記録した本番の値で docs/architecture.md を更新する小さな追加 PR を采配役が作り、CI 通過後にマージする（利用者が承認済み）
