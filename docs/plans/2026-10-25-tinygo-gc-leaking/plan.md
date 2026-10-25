# TinyGo の `-gc=leaking` でリクエストごとの CPU 時間を減らす 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: c0f26957e70588eacf7aed16c00d6af5684f6354
- 作業ブランチ: plan/2026-10-25-tinygo-gc-leaking
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 編成の例外理由: なし
- 計画書の作成担当: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 実装担当: claude-opus-5-5 / high（ビルド引数 1 箇所と計測スクリプトの小さな拡張、docs の数値更新が中心で、設計判断は本計画で済んでいる）
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ実装 claude-fable-5-1 / high、レビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-tinygo-gc-leaking/plan.md と request.md として commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。push・PR 作成・CI 確認は采配役。マージは利用者。マージ後に采配役が本番デプロイと本番の前後比較を行い、本番値を docs に載せる追加 PR を作る（PR 作成と CI 確認まで）。ここまで利用者が承認済み

## 背景・目的

本番の CPU 時間は中央値 30ms で、無料プランの目安 10ms を超えている（打ち切りは出ていない）。手元の計測では、webhook 1 件の処理時間の大半は TinyGo の GC の仕事で、`-gc=leaking`（割り当てるだけで回収しない GC）を付けると webhook の CPU 時間の中央値が約 25ms から約 17ms に下がり、wasm も約 14% 小さくなる。

workers-go はリクエストごとに Go の wasm インスタンスを新しく作って捨てる（`build/worker.mjs` が毎回 `new Go()` と `new WebAssembly.Instance` を行う）ため、回収しないメモリはリクエストをまたいで積み上がらない。1 リクエスト分のメモリが Cloudflare Workers の上限 128MB（isolate 単位、同時処理中のリクエストは合算）に十分収まることを確かめたうえで `-gc=leaking` を既定のビルドにし、docs の説明と数値を合わせる。

## スコープ

- 対象:
  - `scripts/build.sh`: TinyGo のビルドに `-gc=leaking` を付ける（環境変数で外せる切替を含む。未確定事項 1）
  - `scripts/bench-cpu.mjs`: 各場面の wasm メモリの最大値（`memory.buffer.byteLength`）を出す列と、webhook のイベント件数を増やすオプションを足す
  - `docs/development.md`、`docs/architecture.md`: ビルド引数の説明、gzip サイズ、CPU 時間の計測の説明、メモリの上限と最悪ケースの値、切り戻し手順、今後の検討事項
  - `docs/architecture/build-and-deploy.svg`: 図中の gzip サイズの数値だけを変更後の実測値に更新する（図の構成・ほかの文字は変えない）
  - 計測: ベース（`-gc=leaking` なし）と変更後の CPU 時間とメモリの前後比較（手元、Node）、`wrangler dev` と `scripts/local-smoke.mjs` での疎通
- 対象外:
  - `-panic=trap`、`-opt=2`（別に判断する）
  - Go のコードの変更（イベント件数の上限、割り当ての削減など）
  - `tinygo test` への `-gc=leaking` の付与（未確定事項 2）
  - CI の `check-size.sh` の閾値（1024 KiB のまま。目的は Go のビルドへの退避を検出することで、新しい gzip 約 540 KiB でも十分に下）
  - 本番のデプロイと、本番の CPU 時間を docs に載せる追加 PR（公開経路に手順だけ書く）
  - 国データや国旗、README の変更（README は TinyGo の版と導入手順しか書いておらず、変更不要）

## 方針

### ビルド

`scripts/build.sh` の `build_tinygo` で、`tinygo build -o ./build/app.wasm -target wasm -no-debug -gc="${TINYGO_GC:-leaking}" ./cmd/worker` とする。環境変数 `TINYGO_GC` は TinyGo の `-gc` にそのまま渡し、既定を `leaking` にする。値の検証は build.sh では行わず、不正な値は TinyGo のエラーに任せる（`WASM_BUILD` のような分岐がないため）。ファイル先頭の Environment の説明に `TINYGO_GC` を追記する。

TinyGo の wasm ターゲットの既定の GC は precise（TinyGo 0.43.0 同梱の `targets/wasm.json`）で、導入前のビルドは precise。`TINYGO_GC=precise` で導入前と同じ wasm になる（ベース SHA の `npm run build` の `app.wasm` とバイト単位で一致することを確認済み）。

理由は 2 つ。`WASM_BUILD=go npx wrangler deploy` と同じ形で、コードを戻さずに `TINYGO_GC=precise npx wrangler deploy` で前の GC に戻せること。bench の前後比較で、同じ作業ツリーから両方のビルドを作れること。

`-gc=leaking` が安全に使える前提は次の 2 点で、docs に不変条件として書く。

- Go の wasm はリクエストごとに起動して捨てられる（workers-go の生成する `build/worker.mjs` の振る舞い。workers-go の版を上げてインスタンスを再利用する形になったら、`-gc=leaking` を見直す）
- 1 リクエストで使うメモリが小さい（イベント 1 件で約 5 MiB、イベント 20 件で約 40 MiB。上限 128MB は同時処理中のリクエストで合算される）

CI の `build` ジョブは `scripts/build.sh` を経由するため、変更なしで `-gc=leaking` のビルドを作って `check-size.sh 1024` を通す。`tinygo-test` ジョブは変えない（未確定事項 2）。

### メモリの確認方法

`scripts/bench-cpu.mjs` に `mem` 列を常設する。各回の `handle` の後に `instance.exports.memory.buffer.byteLength` を読み、場面ごとの最大値を MiB で表と JSON に出す。読み取りは 1 回 1 プロパティで CPU の計測に影響しない。wasm のメモリは段階的に増えるため、この値は実使用量の上限値である旨を docs に書く。測れるのは wasm の線形メモリだけで、JS 側のヒープは含まない。

最悪ケースのために `--events N`（既定 1）を足す。webhook の「日本」「クイズ」の 2 場面で、同じイベントを N 件（reply token は件ごとに変える）含む 1 リクエストにする。`GET /` は変わらない。本文 5,000 文字の場面は足さない。計画時の計測で 5,000 文字 × 20 件とクイズ × 20 件は同じ 40 MiB で、クイズが最悪ケースの代表になるため（未確定事項 4）。

受け入れの基準値は、単独イベントの場面で 8 MiB 以下、`--events 20` のクイズで 64 MiB 以下（上限の半分。2 リクエストの同時処理まで収まる）。実測値は docs に載せる。

### 複数イベント・同時実行のリスクへの対策

今回はコードで対策しない（未確定事項 3）。理由は、1 対 1 のトークでは通常 1 件で、20 件でも 40 MiB と上限の 3 分の 1 以下に収まること、件数の上限を設けると返信を落とすことになり、CPU 時間を減らすという目的に対して失うものが大きいこと。代わりに次を行う。

- イベント 1 件あたりの増分（約 2 MiB）と 20 件の値を docs に載せ、「同時に処理中のリクエスト数 × 1 リクエストの値が 128MB に近づく使い方（グループで大量に同時に送られるなど）は想定外」と明記する
- 本番で `wrangler tail` の outcome に `exceededMemory` が出ないことを確認項目にする（公開経路）
- `docs/architecture.md` の今後の検討事項に、「イベント件数が多いリクエストの割り当て削減（返信本文・Webhook の解析まわり）」と「件数が上限に近づいたときの打ち切り」を候補として残す

### 代替案と選ばなかった理由

| 代替案 | 選ばなかった理由 |
|---|---|
| `-gc=precise` | TinyGo 0.43 の wasm ターゲットの今の既定で、導入前のビルドそのもの。GC の仕事が残るため CPU 時間が減らない |
| `-gc=none` | 割り当てがあるとリンクエラーで、このプログラムではビルドできない |
| `-gc=leaking -panic=trap` | 最速だが、パニック時の手がかりが消える判断を別に行う（範囲外） |
| `-opt=2` の併用 | 手元で `-opt=2` 単独は効果がなく、wasm が大きくなる。範囲外 |
| イベント件数の上限をコードで設ける | 返信を落とす。実測の余裕が大きいため今回は見送り、候補として記録する |
| メモリ計測を一時的な改変や別スクリプトにする | `-gc=leaking` を使う限りメモリは追い続ける指標になり、bench に常設する方が次回の比較で手間がない |
| GC の切替を設けず `-gc=leaking` を直書きする | 切り戻しが commit の差し戻しか `wrangler rollback` だけになる。環境変数 1 つで前の GC に戻せる方が、既存の `WASM_BUILD` と同じ使い勝手で運用が揃う |

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | 比較用のベース: ベース SHA の `npm run build` で作った `build/app.wasm` と `build/wasm_exec.js` をリポジトリの外に置く（采配役が用意して渡す。`TINYGO_GC=precise` でも作れるが、ベース SHA のビルドを正とする） | - |
| 2 | `scripts/build.sh` の `tinygo build` に `-gc="${TINYGO_GC:-leaking}"` を付け、先頭コメントの Environment に `TINYGO_GC` を追記する | - |
| 3 | `scripts/bench-cpu.mjs` に `mem` 列（場面ごとの `memory.buffer.byteLength` の最大値、MiB）と `--events N` を足し、先頭コメントの Usage と説明を更新する。既存の 3 場面と列の意味は変えない | - |
| 4 | `npm run build` で変更後の wasm を作り、`npm run deploy:dry` で gzip サイズを記録する。`bash scripts/check-size.sh 1024` が通ることを確かめる | 2 |
| 5 | CPU の前後比較: 退避したベースと変更後のビルドを、60 回ずつ交互に 3 巡計測する（タスク 3 の bench を両方に使う。ベースは `--wasm-exec` で退避した wasm_exec.js を指定）。判定は受け入れ基準のとおり | 1, 3, 4 |
| 6 | メモリの計測: 変更後のビルドで `--events 1` と `--events 20` を計測し、`mem` の値を記録する。ベースのビルドでも同じ条件で測り、docs の比較表に載せる | 3, 4 |
| 7 | 疎通: `.dev.vars.example` を使って `npx wrangler dev --ip 127.0.0.1 --port 8787` を起動し、`node scripts/local-smoke.mjs http://127.0.0.1:8787` の 7 場面が通ることを確かめる（workerd 上で `-gc=leaking` のビルドが動く確認） | 4 |
| 8 | docs の更新: development.md（ビルドの仕組み、`TINYGO_GC`、スクリプトサイズの数値、CPU 時間の計測の `mem` 列と `--events`、切り戻し）、architecture.md（指標表の wasm サイズ、制限事項のメモリの不変条件と実測値、今後の検討事項の CPU 時間行の更新と割り当て削減の候補、埋め込み図 `docs/architecture/build-and-deploy.svg` の gzip サイズの数値の更新と `xmllint --noout docs/architecture/build-and-deploy.svg` で SVG として読めることの確認）。本番の CPU 時間の値は変えず、architecture.md に導入前の値である旨の注記を足す | 4, 5, 6 |
| 9 | 検証: `gofmt -l $(git ls-files '*.go')`、`go mod tidy -diff`、`go vet ./...`、`GOOS=js GOARCH=wasm go vet ./...`、`go test -race ./...`、`tinygo test ./internal/...`、`bash scripts/check-size.sh 1024`。Go の変更はないので差分は出ない想定だが、CI と同じ手順で確かめる | 2, 3, 8 |
| 10 | 計画書（plan.md、request.md）を docs/plans/2026-10-25-tinygo-gc-leaking/ に置き、個人環境の情報が含まれないことを確かめて commit する。計測の結果は plan ではなく docs 本文に載せる | 8, 9 |

## 完了条件・受け入れ基準

- [ ] `scripts/build.sh` の TinyGo のビルドが `-gc=leaking` を既定で使う。`TINYGO_GC=precise npm run build` で作った `build/app.wasm` が、ベースのビルドの `app.wasm` とバイト単位で一致する（導入前のビルドに戻せる）
- [ ] `npm run deploy:dry` の gzip サイズが 1024 KiB 未満で、ベース（約 652 KiB）より小さい。実測値を docs に載せる
- [ ] CPU 時間（手元 bench、60 回 × 3 巡、ベースと変更後を交互）: webhook「日本」「クイズ」の両方で、変更後の `total` の中央値が 3 巡すべてでベースの 3 巡の最小値を下回り、巡ごとの中央値の中央値で 20% 以上下がる。`GET /` の `total` はベースの最大値を上回らない。`start`（起動）の中央値は合否に使わない参考値で、ベースと変更後の巡ごとの値を報告する（CPU の減少は `handle` で起きる想定。`start` の中央値の中央値がベースから 20% 以上変わった場合は、その旨を報告に書く）
- [ ] メモリ（手元 bench の `mem`、変更後のビルド）: `GET /`、webhook「日本」「クイズ」の単独イベントがそれぞれ 8 MiB 以下。`--events 20` のクイズが 64 MiB 以下。ベースの値も同じ条件で記録し、docs に比較表を載せる
- [ ] `wrangler dev` と `scripts/local-smoke.mjs` の 7 場面がすべて通る
- [ ] `go test -race ./...`、`tinygo test ./internal/...`、gofmt、`go mod tidy -diff`、`go vet`（ホストと js/wasm）が通る
- [ ] CI の go・tinygo-test・build の 3 ジョブが PR で成功する（build は変更なしで `-gc=leaking` のビルドを作る）
- [ ] docs の整合: development.md と architecture.md のビルド引数・gzip サイズ・CPU 時間の計測の説明・メモリの説明が実装と計測値に一致する。architecture.md の本番 CPU 時間（中央値 30ms）は変えず、「`-gc=leaking` 導入前の値。導入後の本番値は計測後に更新」と注記する。`-gc=leaking` の前提（リクエストごとに起動・破棄、1 リクエストのメモリが小さい）が制限事項に書かれている。図（`docs/architecture/build-and-deploy.svg`）の gzip サイズが architecture.md の指標表・`npm run deploy:dry` の実測値と一致し、`xmllint --noout` が通る
- [ ] `bench-cpu.mjs` の既存の使い方（引数の位置、`--verbose`、`--wasm-exec`）が変わらず、`--events` を省くと従来と同じ場面になる
- [ ] 計画書に個人環境のパス・ユーザー名・私的なリンクが含まれない

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 決定と実装時の扱い |
|---|---|---|
| 1. GC の切替を残すか | 選択肢: (a) `TINYGO_GC`（既定 leaking）で外せる、(b) `-gc=leaking` を直書き。推奨 (a)。切り戻しと bench の前後比較が環境変数だけで済み、`WASM_BUILD` と運用が揃う。費用は build.sh のコメントと development.md の 2〜3 行 | 利用者が (a) に確定。`scripts/check-toolchain-errors.sh` は `WASM_BUILD` の不正値を検証しているが、`TINYGO_GC` の不正値の検証は足さない（TinyGo のエラーに任せる） |
| 2. `tinygo test` に `-gc=leaking` を付けるか | 選択肢: (a) 付けない、(b) 付ける。推奨 (a)。tinygo-test の目的は reflect と標準ライブラリの差の検出で GC は関係ない。ホストのテストは割り当てが多く、回収しない GC では実行中にメモリが増え続ける。本番のバイナリと GC を揃える価値より不安定になるリスクが大きい | 利用者が (a) に確定。CI の tinygo-test ジョブは変更しない |
| 3. イベント件数への対策 | 選択肢: (a) 何もせず docs と本番の監視で扱う、(b) 件数の上限を設けて超過分をログに残して返信しない、(c) 割り当てを減らす。推奨 (a)。1 対 1 では 1 件が通常で、20 件でも上限の 3 分の 1。(b) は返信を落とす。(c) は別計画 | 利用者が (a) に確定。(b)(c) を今後の検討事項に残す。計測で `--events 20` が 64 MiB を超えたら止まって報告する（leaking の採用自体を見直す） |
| 4. bench に 5,000 文字の場面を足すか | 選択肢: (a) 足さず、クイズ × N を最悪ケースの代表にする、(b) `--long-text` などの場面を足す。推奨 (a)。計画時の計測で 5,000 文字とクイズは同じ値で、場面を足すと表と docs が増える | 采配役が推奨の (a) に確定（場面は足さない）。クイズ × 20 がベースの 20 件の値（約 2.5 MiB）と比べて想定（約 40 MiB）から大きく外れたら、原因の切り分けのために手元だけで本文を長くした計測をしてよい（commit しない） |
| 5. bench に `mem` 列を常設するか | 選択肢: (a) 常設、(b) `--memory` で任意。推奨 (a)。費用がほぼなく、`-gc=leaking` を使う間はメモリが追う指標になる | 利用者が (a) に確定。表の幅が大きくなりすぎる場合は MiB 小数 2 桁で収める |
| 6. CPU のぶれで判定できない | 60 回 × 3 巡でも巡ごとの差が 20% を超え、判定が成り立たない | 回数を 120 回に増やして再計測。それでも判定できなければ結果を添えて止まる（基準を緩めない） |
| 7. workerd 上で leaking のビルドが動かない | Node では動くが `wrangler dev` の疎通で失敗する（TinyGo の GC の実装差など） | 止まって報告する。回避のための Go のコード変更は範囲外 |
| 8. 捨てたインスタンスのメモリ解放の遅れ | 捨てた wasm インスタンスの線形メモリは V8 の GC が回収するまで isolate のメモリに残り、連続したリクエストで一時的に合算される。手元では測れない | 計画では対策しない。公開経路の本番確認で `exceededMemory` の有無を見る。出た場合は切り戻す |
| 9. workers-go の版更新 | 将来インスタンスを再利用する形になると leaking は使えない | docs の不変条件として書き、workers-go を上げるときの確認項目にする |
| 10. docs の本番値のずれ | 導入後の本番 CPU 時間は追加 PR まで docs に反映されない | architecture.md に注記を入れ、追加 PR で表と今後の検討事項を更新する |

## 公開経路

1. 実装担当は作業ブランチ plan/2026-10-25-tinygo-gc-leaking に commit するまで。push・PR 作成・CI（go・tinygo-test・build）の確認は采配役。レビューは Codex CLI の read-only で行い、マージは利用者
2. マージ後、采配役が `bash scripts/install-tinygo.sh` を済ませた環境で `npx wrangler deploy` を行う（`build.command` が `npm run build` で `-gc=leaking` のビルドを作る）。デプロイ直後に `npx wrangler tail --format json` を起動し、利用者に実機から前回と同じ要領（国名・クイズ・回答・非テキストを混ぜて 30 件程度）で送ってもらう
3. 本番の確認: tail の記録から CPU 時間（`cpuTime`）の中央値と範囲を出し、前回の「立ち上がり直後の重いリクエスト」と「それ以外」の分け方で前後比較する。outcome に `exceededMemory` と `exceededCpu` がないこと、返信が届くこと（`replied=true`）を確かめる。メモリの数値は本番では見えないため、outcome と Cloudflare のダッシュボードのエラー率で代える
4. 追加 PR: architecture.md の指標表（CPU 時間）と今後の検討事項の CPU 時間行を本番値に更新し、architecture.md の注記を外す。PR 作成と CI 確認で止め、マージは利用者
5. 切り戻し（いずれも采配役が実行、利用者の判断）: 最短は `npx wrangler rollback`（前のデプロイに戻す。静的ファイルは変わっていないので問題ない）。ビルドし直す場合は `TINYGO_GC=precise npx wrangler deploy`（main のまま前の GC で作る）、または `-gc=leaking` 導入前の commit を checkout して `npx wrangler deploy`。切り戻したら docs の既定の説明と実態がずれるため、`TINYGO_GC` の既定を戻す PR を別に出す
