# アーキテクチャ図の TinyGo 対応 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: 98bf180040a173f8497a1b3da87a05ee625efa4c
- 作業ブランチ: plan/2026-10-25-arch-diagram-tinygo
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 編成の例外理由: なし
- 計画書の作成担当: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 実装担当: claude-opus-5-5 / high。文書と手書き SVG の文言差し替えが中心で、箱の幅の判断と表示確認を伴うため通常段
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ実装 claude-fable-5-1 / high、レビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-arch-diagram-tinygo/（plan.md、request.md）として作業ブランチへ commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。検収後に采配役が push・PR 作成を行い、CI 通過後にマージする（マージまで利用者が承認済み）。本番デプロイは対象外

## 背景・目的

wasm のビルドが通常の Go から TinyGo（`npm run build` → `scripts/build.sh`）に切り替わり、CI も go / tinygo-test / build の 3 ジョブ並列になった。`docs/architecture.md` の本文と表は追従済みだが、4 枚の図（`docs/architecture/*.svg`）と、図の alt テキストには旧構成の記述（「Go → app.wasm（2,037 KiB）」「テスト・wasm ビルド・サイズ確認」「中央値 1.18秒」「Go のプログラム」など）が残っている。これらを現在の作りに合わせ、図・alt・本文・表の 4 か所で数値と用語が一致した状態にする。

用語の方針は「Go で書いたプログラムを TinyGo で wasm にビルドする」。「Go をやめた」と読める表現（「TinyGo のプログラム」など）は使わない。

## スコープ

- 対象:
  - `docs/architecture/build-and-deploy.svg`: `<title>`、`aria-label`、「npm run build」箱の説明行、「GitHub Actions（PR）」箱の説明行
  - `docs/architecture/overview.svg`: `<title>`、`aria-label`、worker.mjs 箱の説明行、「Go（app.wasm）」の見出し
  - `docs/architecture/request-timing.svg`: `<title>`、`aria-label`、「返信を送る」行の実測値
  - `docs/architecture/packages.svg`: 変更なしの見込み。本文の表と図の整合を確認するのみ
  - `docs/architecture.md`: 4 枚の図の alt テキスト、全体の構成の段落の 1 文、ビルドとデプロイの段落の末尾 1 文
  - 計画書 2 ファイルの追加（`docs/plans/2026-10-25-arch-diagram-tinygo/`）
- 対象外:
  - 図の構成（箱の配置・矢印・図の種類と枚数）の変更。箱の幅の微調整は、文字がはみ出す場合に限り、右端と矢印の接点を動かさない範囲で許す
  - README の変更（別 PR で行う）
  - `docs/architecture.md` の表・設定・今後の検討事項の内容変更（数値はこの表を正とする）
  - ソースコード・CI・ビルドスクリプトの変更
  - 本番デプロイ

## 方針

### 文言の統一

| 場面 | 表現 |
|---|---|
| 図の中の短い説明 | 「Go の wasm（TinyGo でビルド）」「TinyGo → app.wasm（gzip 652 KiB）」 |
| title / aria-label / alt | 「Go で書いたプログラム（TinyGo で wasm にビルド）」「npm run build で TinyGo により wasm を作り」 |
| CI | 「go・tinygo-test・build の 3 ジョブ並列」。alt では括弧で中身（Go のテスト、TinyGo でのテスト、wasm ビルドとサイズ確認）を補う |
| 数値 | `docs/architecture.md` の表の値をそのまま使う。応答時間 中央値 1.16 秒、wasm gzip 652 KiB。TinyGo の版（0.43.0）は図にも alt にも書かない（版の更新のたびに図を直すことになるため。版は `scripts/toolchain.env` が正） |

各 SVG で `<title>` と `aria-label` は同一文にする（現状の規約）。SVG 内の説明文は和欧間に空白を入れない現状の書式、`architecture.md` の alt は空白を入れる現状の書式をそれぞれ維持する。

### 変更箇所と新しい文言

**build-and-deploy.svg**

| 要素 | 現在 | 変更後 | 幅・位置 |
|---|---|---|---|
| `<title>` と `aria-label` | 「…npm run buildでwasmを作り、wrangler deployで…PRではGitHub Actionsがテストとサイズを確認する。」 | 「Wikidataから国データ、Flagpediaから国旗画像をツールで取得し、Goのコードと画像としてリポジトリに置く。npm run buildでTinyGoによりwasmを作り、wrangler deployでCloudflare Workersに載せる。PRではGitHub Actionsがgo・tinygo-test・buildの3ジョブ（Goのテスト、TinyGoでのテスト、wasmビルドとサイズ確認）を並列に実行する。」 | 表示されないので調整なし |
| `npm run build` 箱の 2 行目（x=428, y=260） | 「Go → app.wasm（2,037 KiB）」 | 「TinyGo → app.wasm（gzip 652 KiB）」 | 概算で約 200px。箱の幅は 196px（x 330〜526）なので、はみ出す可能性が高い。T5 の計測で左右の余白が 8px 未満なら、箱を x=310・width=216 に広げ、箱内 2 行の `<text>` の x を 428 → 418 にする。右端（526）と流入矢印の接点（500,218）は動かさない。左隣の GitHub Actions 箱（右端 266）との間隔は 64 → 44px になる |
| `GitHub Actions（PR）` 箱の 2 行目（x=141, y=260） | 「テスト・wasm ビルド・サイズ確認」 | 「go・tinygo-test・build の 3 ジョブ並列」 | 概算で約 215px。箱の幅は 250px。余白が 8px 未満なら「go・tinygo-test・build の 3 ジョブ」に短縮する（箱は広げない） |

変えない箱の確認: 「public/flags/ PNG 196枚・288KB」の 288KB と、「tools/gencountrygo」の存在は資料から確認できないため T2 で確かめる（後述）。「Go のコードとしてビルドへ」「Go の表」は生成物が Go のソースであることを指しており正しい。

**overview.svg**

| 要素 | 現在 | 変更後 | 幅・位置 |
|---|---|---|---|
| `<title>` と `aria-label` | 「…Workersがリクエストごとに起動するGoのプログラムが、…」 | 「LINEアプリからのメッセージはLINE PlatformからWebhookでCloudflare Workersに届き、Workersがリクエストごとに起動するGoで書いたプログラム（TinyGoでwasmにビルド）が、ローディング表示と返信をLINE Messaging APIに送る。国旗画像はWorkersの静的ファイルからLINEアプリが直接取得する。」 | 調整なし |
| worker.mjs 箱の 2 行目（x=710, y=176、中央寄せ） | 「リクエストごとに Go の wasm を新しく起動」 | 「リクエストごとに Go の wasm（TinyGo でビルド）を新しく起動」 | 概算 約 310px。箱の幅 420px。調整不要の見込み |
| app.wasm 箱の見出し（x=516, y=222、太字、左寄せ） | 「Go（app.wasm）」 | 「Go（app.wasm、TinyGo でビルド）」 | 左寄せで右側に 400px 近い空きがある。調整不要 |

変えない要素: 「国データ 196件と国旗の寸法は Go のコードとして同梱」「1秒待って／返信」「/flags/*.png（196枚）」「シークレット 2つ・変数」は本文の表・設定と一致している。

**request-timing.svg**

| 要素 | 現在 | 変更後 | 幅・位置 |
|---|---|---|---|
| `<title>` と `aria-label` | 「…約1.18秒。…」 | 「Webhookを受けてから200を返すまで約1.16秒。そのうち約1秒は待機で、CPU時間に数えられるのは最初の処理の数十ミリ秒だけ。LINEのタイムアウト2秒まで余裕がある。」 | 調整なし |
| 「返信を送る」行の説明（x=595, y=153） | 「通信 → 200 を返す（実測 中央値 1.18秒）」 | 「通信 → 200 を返す（実測 中央値 1.16秒）」 | 文字数同じ。調整なし |

時間軸は 0.38px/ms で、薄い縦線 x=590 は約 1,184ms に相当する。1.16 秒に合わせるなら x=581 だが、既定では動かさない（未確定事項を参照）。

**packages.svg**

本文の表と図の 5 箱・6 矢印の文言を突き合わせた結果、差分はない。変更しない。ただし図には alt にない矢印「cmd/worker → internal/country（国データを渡す）」があるため、T2 でソース上の依存を確認する。

**docs/architecture.md**

| 箇所 | 現在 | 変更後 |
|---|---|---|
| overview の alt | 「…Workers がリクエストごとに起動する Go のプログラムが、…」 | 「…Workers がリクエストごとに起動する Go で書いたプログラム（TinyGo で wasm にビルド）が、…」（他は現状のまま） |
| overview 直下の段落 | 「Workers はリクエストのたびに Go の wasm を起動し、」 | 「Workers はリクエストのたびに Go の wasm（TinyGo でビルド）を起動し、」 |
| request-timing の alt | 「約 1.18 秒」 | 「約 1.16 秒」 |
| packages の alt | 変更なし | 変更なし |
| build-and-deploy の alt | 「…npm run build で wasm を作り、…PR では GitHub Actions がテストとサイズを確認する。」 | 「Wikidata から国データ、Flagpedia から国旗画像をツールで取得し、Go のコードと画像としてリポジトリに置く。npm run build で TinyGo により wasm を作り、wrangler deploy で Cloudflare Workers に載せる。PR では GitHub Actions が go・tinygo-test・build の 3 ジョブ（Go のテスト、TinyGo でのテスト、wasm ビルドとサイズ確認）を並列に実行する。」 |
| build-and-deploy の段落末尾 | 「デプロイは手元から `wrangler deploy` で行います（CI は PR での確認だけ）。」 | 「デプロイは手元から `wrangler deploy` で行います。CI は go・tinygo-test・build の 3 ジョブを並列に実行して確認するだけで、デプロイはしません。」 |

alt の「Go のコードと画像として」は生成物（`countries_gen.go`、`flagsizes_gen.go`、PNG）を指していて正しいので残す。冒頭の「Cloudflare Workers 上で Go を WebAssembly として動かしています」も正しい（TinyGo は表に明記済み）ので触らない。

### 代替案と選ばなかった理由

- 図から実測値（652 KiB、1.16 秒）を外して表だけに寄せる案: 数値の二重管理は減るが、文字を消すのは「構成を変えない」という決定に抵触しやすく、図単体で見たときの情報も減る。今回は値を更新して残す（未確定事項で選択肢として提示）。
- GitHub Actions 箱を「テスト（Go / TinyGo）・ビルド・サイズ確認」のように役割で書く案: 幅が約 230px で余白が足りず、PR 画面に出るチェック名（go / tinygo-test / build）とも一致しない。ジョブ名で書く案を採る。
- `npm run build` 箱に 3 行目を足して「gzip 652 KiB」を分ける案: 箱の高さ 56px に 11px の 3 行は詰まり、行位置の再計算が必要になる。幅を少し広げる方が差分が小さい。
- TinyGo の版を図に書く案: 版の更新のたびに図を直すことになるので書かない。
- 時間軸の縦線を 1.16 秒の位置に動かす案: 既定では動かさない（未確定事項）。

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | ベース SHA から作業ブランチ plan/2026-10-25-arch-diagram-tinygo を作る。変更前の 4 枚を Chrome ヘッドレスで PNG に出し、`<rect>`・`<path>`・`<line>`・`<text>` の要素数をファイルごとに記録する（比較用、commit しない） | - |
| 2 | 事実の再確認（采配役が計画時に確認済み: `scripts/toolchain.env` の TINYGO_VERSION は 0.43.0、`tools/gencountrygo` は存在、`public/flags/*.png` は 196 枚・合計 288,133 byte、`cmd/worker` は `internal/country` を import、`internal/app` の待機既定は 1000ms）。作業ブランチでも同じであることを確かめ、完了報告に書く | 1 |
| 3 | request-timing.svg の `<title>`・`aria-label`・返信行の `<text>` を変更する | 1 |
| 4 | overview.svg の `<title>`・`aria-label`・worker.mjs 箱・app.wasm 見出しの `<text>` を変更する | 1 |
| 5 | build-and-deploy.svg の `<title>`・`aria-label`・2 つの `<text>` を変更する。変更後に T8 の計測を先行して走らせ、余白 8px 未満なら方針どおり箱の幅（npm run build）または文言（GitHub Actions）を調整する | 1 |
| 6 | packages.svg は T2 の結果を見て変更なしを確定する。cmd/worker → country の依存が無ければ変更せず報告のみ | 2 |
| 7 | docs/architecture.md の alt 4 か所と本文 2 文を変更する | 3, 4, 5 |
| 8 | 機械的な確認: xmllint、grep、title と aria-label の一致、文字を除いた構造の差分、要素数の一致（受け入れ基準の該当項目） | 3, 4, 5, 7 |
| 9 | 表示の確認: 4 枚を PNG に出して変更前と並べて見る。変更した `<text>` の bbox が受け入れ基準の「文字の計測」の基準を満たすことをスクリプトで確かめる | 8 |
| 10 | 計画書 2 ファイルを配置し privacy 検査を通す。対象 6 ファイル（`docs/architecture/overview.svg`、`docs/architecture/build-and-deploy.svg`、`docs/architecture/request-timing.svg`、`docs/architecture.md`、`docs/plans/2026-10-25-arch-diagram-tinygo/plan.md`、`docs/plans/2026-10-25-arch-diagram-tinygo/request.md`）だけを commit する。README と packages.svg に差分がないことを `git status` で確かめる | 2〜9 |

## 完了条件・受け入れ基準

- [ ] 古い記述が残っていない（すべて 0 件）。検索対象は `docs/architecture.md` と `docs/architecture/` に限る（`docs/plans/` は過去と今回の計画書で、変更前の記述を引用するため対象外）:
  ```
  grep -rn "1\.18" docs/architecture.md docs/architecture/
  grep -rn "2,037\|2037" docs/architecture.md docs/architecture/
  grep -rn ">Go → app.wasm" docs/architecture/
  grep -rn "テスト・wasm ビルド・サイズ確認" docs/architecture.md docs/architecture/
  grep -rn "Go の wasm を新しく起動\|Go（app.wasm）<" docs/architecture.md docs/architecture/
  grep -rn "Goのプログラム\|Go のプログラム" docs/architecture.md docs/architecture/
  grep -rn "テストとサイズを確認" docs/architecture.md docs/architecture/
  ```
- [ ] 新しい記述が入っている: `grep -c TinyGo` が overview.svg で 4 以上（title、aria-label、text 2 つ）、build-and-deploy.svg で 3 以上、request-timing.svg と architecture.md で「1.16」がそれぞれ 1 件以上
- [ ] 「TinyGo のプログラム」「TinyGo（app.wasm）」のように Go を書かずに TinyGo を主語にした表現が `docs/architecture.md` と `docs/architecture/` に無い（`grep -rn "TinyGo の\(プログラム\|wasm\)" docs/architecture.md docs/architecture/` が 0 件。`docs/plans/` は対象外）
- [ ] 4 枚すべて XML として読める: `xmllint --noout docs/architecture/*.svg` が成功する
- [ ] 各 SVG で `<title>` の文と `aria-label` の値が完全一致する（xmllint の `--xpath` で両方を取り出して diff）
- [ ] 図の構成が変わっていない: ベース SHA と作業ブランチの各 SVG について、`aria-label` 属性の値と要素の文字列を両方除いたもの（`sed -E 's/aria-label="[^"]*"/aria-label=""/; s/>[^<]*</></g'` を通したもの）を diff し、差分が build-and-deploy.svg の `npm run build` 箱の `x`/`width` と箱内 `<text>` の `x`（広げた場合のみ）に限られる。`<rect>`・`<path>`・`<line>`・`<text>` の要素数が 4 枚とも変更前と一致する
- [ ] packages.svg と README に差分がない（`git diff --stat 98bf180..HEAD` の対象が `docs/architecture/overview.svg`、`docs/architecture/build-and-deploy.svg`、`docs/architecture/request-timing.svg`、`docs/architecture.md`、`docs/plans/2026-10-25-arch-diagram-tinygo/plan.md`、`docs/plans/2026-10-25-arch-diagram-tinygo/request.md` の 6 ファイルだけ）
- [ ] 表示の確認（macOS、Google Chrome ヘッドレス）: 4 枚を次のオプションで PNG にし、変更前の PNG と並べて見て、文字のはみ出し・重なり・箱と矢印のずれが無い。PNG は commit しない
  - overview: `--headless=new --screenshot --window-size=960,440 --hide-scrollbars file://$PWD/docs/architecture/overview.svg`
  - build-and-deploy: `--headless=new --screenshot --window-size=960,300 --hide-scrollbars file://$PWD/docs/architecture/build-and-deploy.svg`
  - packages: `--headless=new --screenshot --window-size=960,300 --hide-scrollbars file://$PWD/docs/architecture/packages.svg`
  - request-timing: `--headless=new --screenshot --window-size=960,230 --hide-scrollbars file://$PWD/docs/architecture/request-timing.svg`
- [ ] 文字の計測: SVG をインラインで埋め込んだ一時 HTML を Chrome ヘッドレス（`--dump-dom`）で開き、変更した `<text>` 要素すべての `getBBox()` を次の基準で確かめる
  - 属する箱: その `<text>` の起点（`x`, `y` 属性の点）を内側に含み、`fill="none"` で枠線（`stroke`）を持つ `<rect>` のうち、面積がいちばん小さいもの
  - 属する箱がある文字: bbox の左右が箱の内側に 8px 以上の余白で収まり、上下が箱の内側に収まる
  - 属する箱がない文字（起点を含む枠線付きの `<rect>` がない。例: request-timing.svg の「通信 → 200 を返す（実測 中央値 1.16秒）」）: bbox が SVG の viewBox の内側に、上下左右とも 8px 以上の余白で収まる
  - どちらの場合も、同じ図の他の `<text>` の bbox と重ならない
  - 計測結果（各 `<text>` の bbox、属する箱の有無と座標、余白）を完了報告に書く
- [ ] architecture.md の alt と対応する SVG の title の意味が一致している（TinyGo・3 ジョブ・1.16 秒の 3 点を 4 枚分照合した結果を完了報告に書く）
- [ ] docs/architecture.md の表の値（1.16 秒、652 KiB、196）と図・alt の値が一致している
- [ ] T2 で確認した事実（toolchain.env の版、gencountrygo の有無、flags の合計サイズ、cmd/worker → country の依存）を完了報告に記載し、図の記述と食い違う場合は未確定事項の扱いに従っている
- [ ] 計画書 2 ファイルに個人環境のパス・ユーザー名・私的なリンクが無い
- [ ] CI: PR 作成後に go / tinygo-test / build の 3 チェックが成功する（文書だけの変更なので影響しない見込み。失敗したら原因を報告し、この PR では直さない）

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| 図に実測値を残すか | 確定: 652 KiB と 1.16 秒を新しい値に更新して図に残す（値は architecture.md の表と同じものを書く） | この方針で進める |
| 時間軸の縦線 | 確定: 薄い縦線 x=590 と「返信を送る」矩形は動かさない（図は概略で、差は軸の約 1.6%、文字と alt に「約」と実測値がある）。文字の実測値だけ 1.16 秒に直す | 動かさない |
| `npm run build` 箱の幅 | 「TinyGo → app.wasm（gzip 652 KiB）」が 196px に収まらない見込み。A: 箱を x=310・width=216 に広げる（右端と矢印は不変）。B: 「TinyGo → wasm（gzip 652 KiB）」に短縮する。推奨 A（app.wasm は本文の生成物表と一致するファイル名） | 計測で余白 8px 未満なら A を適用する。A でも収まらなければ B にして報告する |
| GitHub Actions 箱の文言 | 「go・tinygo-test・build の 3 ジョブ並列」が 250px に対して余白約 18px。収まらなければ「並列」を落とす | 計測結果で担当が判断してよい。箱は広げない |
| 「PNG 196枚・288KB」 | 確定: 采配役が確認済み。public/flags/*.png は 196 枚、合計 288,133 byte（約 288KB）で図と一致 | 変更しない |
| `tools/gencountrygo` | 確定: 采配役が確認済み。tools/ に fetchflags・gencountries・gencountrygo がある | 変更しない |
| cmd/worker → internal/country の矢印 | 確定: 采配役が確認済み。cmd/worker/main.go は internal/country を import している（country.Default() を渡す） | 変更しない |
| フォント差による表示差 | GitHub で見る人の環境により Hiragino / Yu Gothic / 他のフォントに変わり、幅が変わる | 左右 8px 以上の余白を基準にすることで吸収する。余白を確保できない場合は文言短縮を優先する |
| 説明文と図のずれ | alt / title を更新しても図の文字と齟齬が出る | 受け入れ基準の 3 点照合（TinyGo・3 ジョブ・1.16 秒）を 4 枚分行う |
| 数値の鮮度 | 1.16 秒・652 KiB は 2026-10-25 時点。次の計測で古くなる | この PR では表を正とし、表と同じ値を書く。将来の更新手順は計画の範囲外 |
| CI の起動条件 | ci.yml は push to main でも走る。図と本文は「PR」で統一している | この PR では「PR」の表記を維持する（別途の判断事項として補足に記載） |

## 公開経路

作業ブランチ plan/2026-10-25-arch-diagram-tinygo に文書だけの commit を積む。検収後に采配役が push し、main への PR を 1 本作る。PR で CI の 3 ジョブ（go / tinygo-test / build）が走るが、文書変更なので結果に影響しない見込み。マージは采配役が行う。デプロイは手元の `wrangler deploy` で別に行う運用で、この PR のマージでは本番に何も起きない。計画モードでは push・PR・マージを実行しない。
