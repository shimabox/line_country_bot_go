# CI の所要時間短縮（TinyGo 導入後） 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: 7d002f370a90e1d269432380cae592f4dce0c3d5
- 作業ブランチ: plan/2026-10-25-ci-speed
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 編成の例外理由: なし
- 計画書の作成担当: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 実装担当: claude-opus-5-5 / high（変更対象は workflow と shell スクリプトと README の数行。設計は本計画で確定しているため通常段で足りる）
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ実装 claude-fable-5-1 / high、レビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-ci-speed/ の plan.md と request.md を commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。検収後に采配役が push・PR 作成・CI の再実行（所要時間の確認）を行い、受け入れ基準を満たせばマージまで、利用者が承認済み。古い Actions キャッシュの削除は采配役の任意。本番デプロイは行わない

## 背景・目的

TinyGo を導入した PR の後、CI（`.github/workflows/ci.yml` の test ジョブ 1 つ）の所要時間が約 47 秒から約 4 分 40 秒（キャッシュあり）に伸びた。計測から、増分の大半は次の 3 つに集中している。

| 原因 | 秒数（キャッシュあり） | 内容 |
|---|---|---|
| `gofmt -l .` | 86〜89 | `.tools/` に展開された TinyGo 同梱の .go 1,862 個まで走査している。対象の .go は git 管理下の 44 個 |
| Build と Script size の二重ビルド | 42〜44 | `scripts/check-size.sh` の `wrangler deploy --dry-run` が `build.command`（`npm run build`）を実行し、直前の Build step と同じ TinyGo ビルドをもう一度行っている |
| `tinygo test ./internal/...` | 68〜73 | TinyGo はリンク後のモジュール全体を最適化するため、標準ライブラリのキャッシュがあってもテストバイナリのコンパイルに時間がかかる。短縮しにくい |

目的は、検査の内容（gofmt・tidy・vet・race テスト・js/wasm vet・tinygo test・TinyGo ビルド・サイズ検査）を一つも落とさずに、キャッシュありの壁時計時間を 2 分 30 秒以下にすること。不要な走査と重複ビルドをなくし、短縮できない `tinygo test` は他の検査と並列に走らせて壁時計時間から隠す。

## スコープ

- 対象:
  - `.github/workflows/ci.yml`: ジョブを 3 つ（Go の検査 / TinyGo のテスト / ビルドとサイズ検査）に分けて並列化する。gofmt の対象を git 管理下の .go に限る。TinyGo ビルドを 1 回にする。`~/.cache/tinygo` のキャッシュキーをジョブごとに分ける
  - `scripts/check-size.sh`: wrangler が失敗したときに出力が捨てられる既存の問題を直す（ビルドをこの step に集約するため、失敗時の出力が見える必要がある）
  - `README.md`: 「ビルドと開発」のコマンド一覧に gofmt の確認コマンドを 1 行足す。CI に触れている既存の記述は内容が変わらないので直さない
  - `docs/plans/2026-10-25-ci-speed/plan.md` と `request.md`
- 対象外:
  - `scripts/build.sh`・`scripts/install-tinygo.sh`・`scripts/toolchain.env`・`wrangler.jsonc`・`package.json` の変更
  - `tinygo test` の最適化レベル（`-opt`）やパッケージ並列実行による短縮（検査内容が変わる可能性があり、並列ジョブで壁時計時間から隠れるため今回は採らない）
  - setup-go のキャッシュ設定の変更（効果が 5〜10 秒程度と見込まれ、計測なしに触らない）
  - 同一 PR への連続 push 時の実行取り消し（`concurrency`）
  - アーキテクチャ図の TinyGo 化と README の整理（別 PR）
  - 検査項目の追加・削除、Go や TinyGo の版の変更

## 方針

### 1. gofmt の対象を git 管理下の .go に限る

`gofmt -l .` を次に置き換える。

```sh
unformatted="$(gofmt -l $(git ls-files '*.go'))"
```

`git ls-files` の pathspec `*.go` は階層を問わず一致するため、44 個の .go を列挙する。`.tools/` は `.gitignore` で除外され、`build/` と `node_modules/` も同様に含まれない。Go のソースファイル名に空白はないので単純な展開でよい。README にも同じコマンドを載せ、手元でも CI でも同じ検査になる。

分割後の Go ジョブには `.tools` のキャッシュを復元しないため `gofmt -l .` でも速いが、手元で同じ問題が起きること、将来ジョブ構成を変えても壊れないことから、対象の明示を採る。

選ばなかった代替案:
- CI だけ `TOOLS_DIR` をリポジトリ外（例: `~/.tools`）にする: 手元の `gofmt -l .` が遅い問題は残り、CI と手元で TinyGo の置き場所が変わる
- `gofmt -l cmd internal tools`: 新しいディレクトリや直下の .go を見落とす
- `gofmt -l $(go list -f '{{.Dir}}' ./...)`: `cmd/worker` が js/wasm 専用で `go list` が警告を出すなど、扱いが複雑になる

### 2. TinyGo ビルドを 1 回にする

Build step の `npm run build` を削り、`scripts/check-size.sh 1024` だけを実行する。wrangler の dry-run は `build.command` 経由で `npm run build`（`scripts/build.sh`）を実行するので、ビルドの内容は現在の Build step と同じで、gzip サイズはそのビルドの結果から読まれる。dry-run の後に `build/app.wasm`・`build/worker.mjs`・`build/wasm_exec.js` の存在確認（現在の Build step の `test -f`）を続けて行い、Build step の検査内容を保つ。

このとき `check-size.sh` の既存の問題を直す。現在は `out="$(npx wrangler ... 2>&1)"` を `set -e` の下で実行しているため、wrangler（＝ビルド）が失敗すると `echo "$out"` に到達せず、失敗の理由が出力されない。終了コードを受け取ってから出力を表示し、非 0 ならその終了コードで終了するように変える。成功時の出力・引数・既定値（2560 KiB）・gzip の読み取り方は変えない。手元での使い方は現在と同じ（`bash scripts/check-size.sh 1024` でビルドしてサイズを測る）。

選ばなかった代替案:
- `scripts/build.sh` に「`build/` が揃っていればビルドを省く」環境変数を足し、Script size step でそれを使う: Build step のログが見やすく手元でも省時間になるが、環境変数を shell に残したまま `npx wrangler deploy` を実行すると古い `build/` をデプロイする危険がある。短縮効果は同じなので、新しい仕組みを増やさない案を採る
- wrangler を使わず `dist/` を自前で gzip してサイズを測る: wrangler が報告する値（実際の上限判定に使われる値）と一致しなくなる

### 3. ジョブを 3 つに分けて並列にする

| ジョブ id | 内容 | 必要な setup | 見込み（キャッシュあり） |
|---|---|---|---|
| `go` | gofmt、`go mod tidy -diff`、`go vet ./...`、`go test -race ./...`、`GOOS=js GOARCH=wasm go vet ./...` | checkout、setup-go | 約 40 秒 |
| `tinygo-test` | `.tools` と `~/.cache/tinygo` の復元、`scripts/install-tinygo.sh`、Tool versions、`tinygo test ./internal/...` | checkout、setup-go、cache ×2 | 約 1 分 55 秒 |
| `build` | `.tools` と `~/.cache/tinygo` の復元、`scripts/install-tinygo.sh`、Tool versions、`npm ci`、`scripts/check-size.sh 1024`、`build/` の 3 ファイルの存在確認 | checkout、setup-go、setup-node、cache ×2 | 約 1 分 40 秒 |

ジョブ間に `needs` は付けない（付けると直列化して遅くなる）。壁時計時間はいちばん遅い `tinygo-test` に支配され、約 2 分と見込む。合計の実行時間（3 ジョブの和）は現在と同程度で、増えない。

2 ジョブ（Go / TinyGo）案は TinyGo 側が `tinygo test` 73 秒とビルド 46 秒を直列に持つため約 2 分 30 秒になり、3 ジョブ案より 40 秒ほど遅い。ジョブを分けない案は、1 と 2 の短縮だけで約 2 分 50 秒にとどまる。

ジョブ id と表示名は上の 3 つにする。現在の `test` という名前は残らない。リポジトリは非公開・無料プランでブランチ保護が使えないため、必須チェックの更新は不要。

利用者が確定: 3 ジョブで並列にする（1 ジョブのまま無駄だけを省く案は待ち時間 約 2 分 50 秒、2 ジョブ案は 約 2 分 30 秒。非公開リポジトリの Actions の無料枠は、ジョブごとに分単位で切り上げて消費され、3 ジョブでも 1 回あたり約 5 分で現在と同程度）。

### 4. キャッシュの扱い

- `.tools`（TinyGo と Binaryen）: キーは現在どおり `tinygo-${{ runner.os }}-${{ hashFiles('scripts/toolchain.env') }}`。`tinygo-test` と `build` の両方で復元する。キーが無いときは両ジョブが取得して保存を試み、後から保存した側は「既に存在する」警告になるが無害
- `~/.cache/tinygo`（TinyGo のビルドキャッシュ）: ジョブごとに別のキーにする。`tinygo test` はホスト向け、`tinygo build` は wasm 向けの中間生成物を作り、内容が異なる。同じキーを共有すると、先に終わったジョブの内容だけが保存され、もう一方のジョブは毎回標準ライブラリを作り直すうえ、キーが存在するため保存もされない。キーは `tinygo-cache-${{ runner.os }}-<test|build>-${{ hashFiles('scripts/toolchain.env', 'go.sum') }}` とし、現在ハードコードされている `0.43.0` を `toolchain.env` のハッシュに置き換える。`restore-keys` に `tinygo-cache-${{ runner.os }}-<test|build>-` を置き、`go.sum` が変わっても直前のキャッシュから標準ライブラリ分を再利用する
- setup-go のキャッシュ（`~/.cache/go-build` と module cache）は 3 ジョブとも既定のまま使う。`build` と `tinygo-test` でも `go run .../workers-assets-gen` と TinyGo のモジュール解決が module cache を使う
- setup-node（`cache: npm`）は `build` ジョブだけに置く
- キーの名前が変わるため、この変更後の最初の実行はキャッシュなし（コールド）になる。計測は 2 回目以降で行う。古いキー（`tinygo-cache-Linux-0.43.0-...`）のキャッシュは使われなくなるので、采配役が任意で削除してよい

### 不変条件

- 現在の 10 の検査（gofmt、tidy、vet、race テスト、js/wasm vet、tinygo test、TinyGo ビルド、ビルド成果物 3 ファイルの存在、Tool versions、gzip 1024 KiB）はどれかのジョブに必ず含まれ、コマンドの引数（`-race`、`./...`、`./internal/...`、`1024`）は変えない
- トリガー（`push` の main と `pull_request`）と `permissions: contents: read` は変えない
- `scripts/build.sh`・`install-tinygo.sh`・`toolchain.env` は変えないので、手元のビルドとデプロイの手順は変わらない

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | 作業ブランチ plan/2026-10-25-ci-speed をベース SHA から作り、docs/plans/2026-10-25-ci-speed/ に plan.md と request.md を置く | - |
| 2 | `scripts/check-size.sh`: wrangler の終了コードを受け取ってから出力を表示し、非 0 ならその終了コードで終了するように直す。先頭コメントに「失敗時も出力を表示する」ことを足す | 1 |
| 3 | `.github/workflows/ci.yml`: `go` ジョブを作る（checkout、setup-go、gofmt を `git ls-files` 版に、tidy、vet、race テスト、js/wasm vet）。コメントは英語のまま内容を更新する | 1 |
| 4 | `.github/workflows/ci.yml`: `tinygo-test` ジョブを作る（checkout、setup-go、`.tools` と `~/.cache/tinygo`（test キー）の復元、install、Tool versions、`tinygo test ./internal/...`） | 3 |
| 5 | `.github/workflows/ci.yml`: `build` ジョブを作る（checkout、setup-go、setup-node、`.tools` と `~/.cache/tinygo`（build キー）の復元、install、Tool versions、`npm ci`、`bash scripts/check-size.sh 1024`、`build/` の 3 ファイルの `test -f`）。旧 `test` ジョブを削除する | 3 |
| 6 | `README.md` の「ビルドと開発」のコマンド一覧に `gofmt -l $(git ls-files '*.go')` を 1 行足す（.tools/ を除くこと、CI と同じであることを添える） | 3 |
| 7 | 手元検証（受け入れ基準の「手元で確認」の項目を実行し、結果を完了報告に残す） | 2, 3, 4, 5, 6 |
| 8 | 作業ブランチへ commit（計画書は privacy 検査後に含める）。CI の計測は push 後に采配役が行う | 7 |

## 完了条件・受け入れ基準

検査の網羅性（workflow の内容で確認）:
- [ ] `ci.yml` に `go`・`tinygo-test`・`build` の 3 ジョブがあり、`needs` が無い
- [ ] 次の検査がいずれかのジョブに同じ引数で含まれる: gofmt（git 管理下の .go が対象）、`go mod tidy -diff`、`go vet ./...`、`go test -race ./...`、`GOOS=js GOARCH=wasm go vet ./...`、`tinygo test ./internal/...`、`bash scripts/check-size.sh 1024`、`build/app.wasm`・`build/worker.mjs`・`build/wasm_exec.js` の存在確認、`tinygo version` と `wasm-opt --version`
- [ ] `~/.cache/tinygo` のキャッシュキーが `tinygo-test` と `build` で異なり、`scripts/toolchain.env` と `go.sum` のハッシュを含み、`restore-keys` がある。`0.43.0` のハードコードが無い
- [ ] `.tools` のキャッシュキーは現在と同じ
- [ ] トリガーと `permissions` は変わっていない

手元で確認（実装担当）:
- [ ] `gofmt -l $(git ls-files '*.go')` が 1 秒以内に終わり出力が空。任意の .go の整形を一時的に崩すとそのファイル名が出力される（確認後に戻す）
- [ ] `bash scripts/install-tinygo.sh` 済みの環境で `bash scripts/check-size.sh 1024` が成功し、出力の最後に `check-size: gzip <値> KiB (limit 1024 KiB)` が出て、`build/` に 3 ファイルがある
- [ ] `WASM_BUILD=foo bash scripts/check-size.sh 1024` が非 0 で終了し、wrangler とビルドのエラー出力（`build: unknown WASM_BUILD=foo`）が表示される（修正前は何も表示されずに終了する）
- [ ] `bash scripts/check-toolchain-errors.sh` が全件 ok
- [ ] `go test -race ./...` と `tinygo test ./internal/...` が手元で成功（変更の影響確認。コマンド自体は変えていない）
- [ ] shellcheck と actionlint が手元にあれば `scripts/check-size.sh` と `.github/workflows/ci.yml` に警告が無い。無ければ未実行と報告する
- [ ] `git status` で変更ファイルが `.github/workflows/ci.yml`、`scripts/check-size.sh`、`README.md`、`docs/plans/2026-10-25-ci-speed/` だけ

CI で確認（采配役が push 後に行う。検収の基準）:
- [ ] PR の最初の実行（キャッシュなし）で 3 ジョブすべてが成功する
- [ ] 所要時間の判定（対象はキャッシュありの実行。最初のコールドの実行は参考値で、回数に数えない）: 同じ SHA で「Re-run all jobs」を 2 回行い、ワークフロー全体の壁時計時間（Actions の run ページに表示される duration、または `gh run view` の開始〜終了）が 2 回とも 2 分 30 秒（150 秒）以下なら合格。2 回のうち 1 回だけ 150 秒を超えた場合は、同じ SHA でもう 1 回だけ再実行し、3 回のうち 2 回以上が 150 秒以下なら合格。比較の基準は変更前の約 4 分 40 秒〜5 分 17 秒
- [ ] 2 回目以降の実行で、両 TinyGo ジョブの `~/.cache/tinygo` の復元がキー一致（`Cache restored from key: ...-test-...` / `...-build-...`）になっている
- [ ] 2 回目以降の実行で gofmt step が 5 秒以下、`build` ジョブの check-size step が TinyGo ビルド込みで 60 秒以下（dry-run 1 回分）
- [ ] step ごとの秒数を変更前の表と同じ形で記録し、完了報告に残す
- [ ] 150 秒を超えた回が 2 回以上になった時点で不合格とし、どのジョブが律速か（通常は `tinygo-test`）と各 step の秒数を報告して止まる。追加の短縮は別途判断する

手元の使い勝手:
- [ ] `npm run build`、`npm run deploy:dry`、`bash scripts/check-size.sh`（引数省略時の上限 2560 KiB）、`npx wrangler deploy` の挙動が変わっていない（`build.sh` と `wrangler.jsonc` は未変更）

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| ブランチ保護の必須チェック | 確定: リポジトリは非公開・無料プランで、ブランチ保護とルールセットは使えない（API が 403 を返す）。ジョブ名が `test` から `go`・`tinygo-test`・`build` に変わってもマージは止まらない | 対応不要 |
| ビルド集約の方式 | 確定: check-size.sh の dry-run に集約する（スクリプトの改修は失敗時の出力表示だけ）。build.sh に `build/` を再利用する環境変数を足す案は、環境変数を残したままデプロイすると古い成果物を載せる危険があるので採らない | 推奨どおり |
| check-size.sh の出力が末尾にまとまる | dry-run の出力を変数に集めて後から表示するため、CI のログでは約 45 秒間出力が無い。失敗時は修正により出力される | 許容する。ストリーミング表示（`tee`）への変更は不要。実装担当は変えない |
| `tinygo test` の所要時間 | 73 秒は並列化で隠すだけで短くならない。`-opt=1` などは検査内容が変わる可能性があるので採らない。ホストが 4 vCPU のため複数パッケージの並列実行の効果も不明 | この PR では扱わない。所要時間は受け入れ基準の「CI で確認」の判定（ワークフロー全体の壁時計時間で判定）に従い、不合格ならどのジョブが律速か（通常は `tinygo-test`）と各 step の秒数を報告して止まる |
| キャッシュの保存競合 | `.tools` のキーが無いとき、2 ジョブが同時に取得・保存を試みる。片方の保存が「already exists」警告になるが失敗ではない | 警告は許容する。2 回目以降の実行でどちらのジョブも復元できていればよい |
| コールドスタート | キーの改名でこの変更後の最初の実行はキャッシュなし。`tinygo test` は標準ライブラリの作り直し（約 14 秒）が足される。main へのマージ後も main 側の最初の実行はコールド | 計測は 2 回目以降で行う。コールドの所要時間は参考値として記録する |
| gofmt の対象が git 管理下に限られる | 手元で未追跡の .go は検査されない（CI では checkout した commit がすべて追跡済みなので差は無い）。生成ファイル（`countries_gen.go` など）は追跡されているので引き続き検査される | 手元で未追跡ファイルも含めたい場合は `git ls-files -co --exclude-standard '*.go'` を使えることを README に書くかは実装担当の判断（書かなくてよい） |
| 所要時間の見込みの誤差 | 各ジョブの setup（checkout、setup-go、キャッシュ復元）は 20〜35 秒と見込むが、ランナーの負荷で変わる。目標 2 分 30 秒には約 30 秒の余裕を取っている | キャッシュありの実行（最初のコールドの実行は回数に数えない）を同じ SHA で 2 回計測し、2 回とも 150 秒以下なら合格。1 回だけ 150 秒を超えた場合は同じ SHA でもう 1 回だけ再実行し、3 回のうち 2 回以上が 150 秒以下なら合格。150 秒を超えた回が 2 回以上になった時点で不合格とし、どのジョブが律速か（通常は `tinygo-test`）と各 step の秒数を報告して止まる。追加の短縮は別途判断する |
| 古いキャッシュの削除 | 旧キー（`tinygo-cache-Linux-0.43.0-...`、旧ジョブで保存した setup-go のキャッシュ）は参照されなくなる。放置しても 7 日で失効する | 実装担当は触らない。采配役が任意で削除する |

## 公開経路

1. 実装担当が作業ブランチ plan/2026-10-25-ci-speed に commit する（ここまでが実装担当の範囲）
2. 采配役が push し、main 向けの PR を作る。PR の `pull_request` トリガーで新しい 3 ジョブが実行される（1 回目はコールド）
3. 采配役が同じ SHA で「Re-run all jobs」を 2 回行い、受け入れ基準の所要時間を確認する。1 回だけ 150 秒を超えた場合は、同じ SHA でもう 1 回だけ再実行する
4. 実装レビュー（Codex CLI, read-only）と検収の後、采配役がマージする。main の `push` トリガーで実行され、main 側のキャッシュが作られる
5. 本番デプロイは無い。CI は検査だけで、デプロイは引き続き開発者の手元の `npx wrangler deploy`。この PR は `build.sh` と `wrangler.jsonc` を変えないのでデプロイ手順に影響しない
