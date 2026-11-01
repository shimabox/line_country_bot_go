# 首都クイズと首都名検索 実装計画

- 日付: 2026-11-01
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: a62c3db2b6b5e2dc21b875e5085c20bd9f06984a
- 作業ブランチ: plan/2026-11-01-capital-quiz
- 起案担当: claude-fable-5-1 / high（確認済み: 実行結果のメタデータで claude-fable-5-1。effort は未確認）
- 計画レビュー担当: 予定 gpt-6-astra / high
- 文書化担当: claude-opus-5-5 / medium
- 編成の例外理由: なし
- 実装担当: claude-opus-5-5 / high（Claude Code 環境の通常段）
- 実装レビュー担当: 予定 gpt-6.1-sol / high
- 計画書の扱い: docs/plans/2026-11-01-capital-quiz/plan.md と request.md を作業ブランチに commit し、PR に含める
- 公開範囲: 公開リポジトリ。個人環境の情報は書かない

## 背景・目的

bot は国名か別名を送ると国旗と首都を返し、「クイズ」で国旗から国名を当てる 4 択を出す。首都のデータは 196 か国すべてにあるが、検索と出題には使っていない。首都を使った遊び方を 2 つ足す。

1. 「クイズ」「quiz」を送ると、既存の「国旗→国名」と新しい「国旗→首都」のどちらかがランダムに出る。首都クイズの選択肢は首都名のボタン 4 つ。
2. 首都名（「東京」「Tokyo」）で国を検索できる。首都名が自国の国名や別名と同じ国（シンガポールなど）は国として返す。

既存の国旗クイズと国名検索の見た目と文言は変えない。状態を持たない設計（KV / D1 なし）と、起動時に索引を作らない設計（生成コード）を守る。

## スコープ

### 対象

- internal/country: 首都名（capital_ja と capital_en）を検索索引に追加する。国コードで国を引く関数と、入力が国名・別名に一致したかを判定する関数を追加する。
- tools/gencountrygo と countries_gen.go: 索引の仕様変更に伴う再生成と、生成器のテスト更新。
- internal/bot: 首都クイズの出題と回答、クイズ種類のランダム選択、首都検索ヒット時の返信、postback データの拡張と旧形式の互換。
- scripts/local-smoke.mjs: 首都検索、首都クイズ、旧形式 postback の場面追加。
- docs: README.md の機能表、docs/architecture.md、docs/development.md、docs/data-update.md。
- CPU 時間と wasm サイズの計測。

### 対象外

- 送る言葉の追加（「首都クイズ」のような専用トリガー）。利用者の決定により今回は入れない。
- 「・」や「.」を除いた表記（「アンドララベリャ」）の受け付け。NormalizeKey の変更は国名索引にも影響するため別 PR の候補とする。
- 国旗クイズの文言・メッセージ数・不正解時の挙動の変更。
- data/countries.json の内容変更。
- 旧形式 postback の互換の打ち切り。

## 方針

### 1. 首都名検索の索引

- BuildIndex を 3 段階にする。優先順位は **国名 > 別名 > 首都名**。
  - 第 1 段（国名）と第 2 段（別名）は現状のまま。
  - 第 3 段で各国の capital_ja と capital_en を NormalizeKey したキーを候補にする。既に国名・別名のキーとして存在するものは入れない（他国の国名・別名が勝つ。自国と同じなら結果は同じ）。首都名どうしで異なる国が同じキーになるものは曖昧として入れない。
  - 首都名の衝突はエラーにしない。別名と同じ寛容さで扱う。国名どうしの衝突だけが従来どおりエラー。
- 索引は既存の countryIndex に混ぜる。Lookup は変更不要で、二分探索 1 回のまま。
- capital_en も索引に入れる（Tokyo で日本が出る）。国名検索が英語別名を受けるのと揃える。
- countries_gen.go を `go generate ./internal/country` で再生成する。鮮度テスト（internal/country/generated_test.go の TestGeneratedMatchesJSON。data/countries.json を Load と BuildIndex で読んだ結果と、生成済みの国表・索引が一致するかを比べる）が再生成前は失敗し、再生成後に通ることを TDD の順序に組み込む。
- データ不変条件のテストを追加する。bundled データの全国について、capital_ja と capital_en を Lookup すると自国に当たること。将来のデータ更新で他国と衝突すればこのテストで気付ける。

### 2. 首都検索ヒット時の返信

- Lookup の戻り値だけでは「国名で当たった」か「首都名で当たった」かが分からない。Country に、正規化済みキーが国名または別名のいずれかに一致するかを返すメソッド（例: `NamedBy(key string) bool`）を足し、bot 側で判定する。索引の構造は変えない。
- 首都名で当たったとき（国名・別名に一致しない）:
  - 1 通目のエコーは現状どおり「[東京] ですね🤔」。
  - 2 通目は現状どおり「わかります😀」。
  - 3 通目の詳細は **タイトルを国名（NameJA）** にし、本文は現状どおり「東京 / Tokyo」、国旗画像つき。入力をタイトルにすると「東京」の下に「東京 / Tokyo」と並び国名が出ないため。
  - ログ用 Kind は `KindCapitalHit` を新設する。国名ヒットは KindCountryHit のまま。
- 国名・別名で当たったときは現状どおり入力をタイトルにする（既存テスト TestCountryHitEnglishUsesInputAsTitle を維持）。シンガポールのように首都名が国名と同じ入力は、この分岐により国名ヒットになる。

### 3. クイズ種類のランダム選択

- 割合は **1:1**。乱数源 `b.rand` から最初に 1 回 `IntN(2)` を取り、種類を決めてから国を選ぶ。
- 出題処理を `quizFlag` と `quizCapital` に分け、`quiz` は種類を決めて振り分けるだけにする。テストは同一パッケージなので両関数を直接呼んで決定的に検証でき、Respond 経由ではシードを 30 個程度回して両種類が出ることと不変条件を確かめる。
- 首都クイズのフォールバック: 選んだ 4 か国のいずれかの capital_ja が空、または首都名のボタンラベルが重複する場合は国旗クイズに切り替える。現データでは起きないが、データ更新時の防御として入れ、Load で作ったフィクスチャ Store で検証する。
- 乱数の消費順が変わる。Respond 経由で国旗クイズを前提にしている TestQuiz と TestQuizShufflesCorrectPosition は quizFlag の直接呼び出しに移す（タスク 7）。それ以外の既存テストは固定シードで同じ結果になること（TestDeterministicWithSeed）と不変条件を見ており、特定の出題を固定していない。

### 4. 首都クイズの出題

- ButtonsTemplate で、国旗クイズと同じ画像設定（flagImage）。
  - Title: 「クイズです」（共通）
  - Text と AltText: 「この国の首都はどこでしょうか？」
- 選択肢 4 つは正解国と他 3 か国の capital_ja。ラベルは TruncateUTF16 を通す（最長 14 文字で上限 20 に収まる）。DisplayText は capital_ja。
- 正解の位置はシャッフルする（国旗クイズと同じ）。
- Reply.Code は正解国のコード。ログ用 Kind は `KindCapitalQuiz` を新設する（国旗クイズは KindQuiz のまま）。

### 5. postback データの形（互換を含む主要な契約）

- 現状: `answer=<国名>&result=<0|1>`。
- 新形式: 既存キーに `code=<ISO 2 文字>` を加える。首都クイズはさらに `q=capital` を付ける。url.Values.Encode はキーをソートするので実際の並びは `answer, code, q, result` の順になる。
  - 国旗クイズ: `answer=<国名>&code=<ISO>&result=<0|1>`
  - 首都クイズ: `answer=<首都名>&code=<ISO>&q=capital&result=<0|1>`
- code はボタンの国（押した選択肢の国）のコード。正解ボタンでは正解国と同じ。不正解時に正解を教えない方針のため、正解国のコードを不正解ボタンに乗せる必要はない。
- 長さ: 首都名の最長 14 文字は URL エンコードで 126 文字。他のキーを足しても 170 文字未満で、上限 300 に収まる。国名側は既存テスト（MaxPostbackData 以下）で引き続き確認する。
- respondAnswer の国特定は次の順:
  1. `answer` が空、または `result` が 0/1 以外、またはパースエラーなら無視（現状どおり）。
  2. `code` があれば `Store.ByCode` で国を引く。見つからなければ詳細を省いて確認だけ返す（現状の「データ更新で名前が消えた」ときと同じ扱い）。
  3. `code` がなければ旧形式として `ByName(answer)` で引く（現状どおり）。
- **デプロイ前に出したボタン**は code がないので 3 の経路で現状と同じ返信になる。互換は保つ。ただし旧ボタンに `q` はないので、常に国旗クイズの回答として扱われる（旧ボタンは国旗クイズしかないので正しい）。
- `code` の形式検査は validCode 相当（大文字 2 文字）。満たさないときは `code` がないものとして扱う。
- 1 通目のエコーは `answer` をそのまま使う（首都クイズなら「[東京] ですね?」）。
- ログ用 Kind は `q=capital` のとき `KindCapitalAnswerCorrect` / `KindCapitalAnswerWrong` を使い、それ以外は既存の Kind。本番ログで首都クイズの利用を見分けるため。

### 6. 首都クイズの正解・不正解の返信

- 正解: 「[東京] ですね?」、「む、むむっ、、\n正解です」+絵文字、国の詳細（タイトル=国名、本文=首都、国旗なし）、「もう一度やりますか？」。国旗クイズの正解と同じ 4 通構成。
- 不正解: 「[パリ] ですね?」、「む、むむっ、、\n残念！はずれです」+絵文字。国旗クイズと同じ 2 通構成で、正解は教えない。
- 「はい」は「クイズ」を送るので、再出題の種類はまたランダムになる。
- 返信は常に 5 通以下。TestReplyCountNeverExceedsLimit に首都の入力と postback を加える。

### 7. CPU 時間と wasm サイズ

- 索引は約 380 エントリ増える（自国の国名・別名と同じ 14 件は除外）。未圧縮で約 15 KiB、gzip 後は数 KiB の見込み。コード追加を含めても gzip で **+10 KiB 以内** を見込む。
- 受け入れ上限: gzip サイズ 600 KiB 以下（現在約 553 KiB）。scripts/check-size.sh の引数 1024 は変えない。
- CPU: 起動時に索引を作らないので変化なし。索引が増えても Lookup の二分探索の比較は 1 回増える程度、クイズは乱数 1 回分増える程度。bench-cpu の「日本」「クイズ」の中央値が現状比 +10% 以内であることを確認する（Node の計測はぶれが大きいため、超えたときは 3 回計測し、その中央値で判断する）。bench-cpu の場面に首都検索（東京）を足す。

### 選ばなかった代替案と理由

- **首都索引を別配列にする**: Lookup を 2 回引く形になり、CPU とコードが増える。首都でヒットしたかは NamedBy で判定できるので、混ぜる方が単純。
- **IndexEntry に種別フィールドを持たせる**: 生成コードと生成器の変更が大きい。種別が必要なのは bot のタイトル判定だけで、国名・別名の件数は少なく再比較で足りる。
- **postback に国名ではなく code だけを乗せる**: エコーに表示する名前が必要で、旧形式との判定も複雑になる。answer は表示用、code は特定用と役割を分ける。
- **首都クイズの postback を別キー名（capital=）にする**: respondAnswer の分岐が増え、エコー処理を二重に持つことになる。`q=capital` の 1 キーで足りる。
- **常にタイトルを国名にする**: 英語入力でタイトルが入力になる現行挙動を変えてしまう。
- **NormalizeKey で「・」「.」を除く**: 国名索引の意味が変わり、既存の別名衝突判定も変わる。別 PR の候補とする。
- **専用トリガー「首都クイズ」を足す**: 利用者の決定（送る言葉は今のまま）に反する。

## タスク分解（TDD の順）

| # | タスク | 依存 |
|---|---|---|
| 1 | 采配役が用意した作業ブランチで、配置済みの plan.md と request.md を最初の commit にする | なし |
| 2 | internal/country: 索引テストを追加（Tokyo・東京→JP、シンガポール→SG、首都名が他国の国名・別名と一致するフィクスチャで国名・別名が勝つ、首都名どうしの重複は曖昧除外、首都名が別名と衝突すれば別名優先、全国の首都名が自国に当たる不変条件）。失敗を確認 | 1 |
| 3 | BuildIndex に第 3 段（首都名）を実装し、テストを通す | 2 |
| 4 | tools/gencountrygo のテストに首都キーが生成されることを追加し、`go generate ./internal/country` で countries_gen.go を再生成。鮮度テストと生成器テストが通ることを確認 | 3 |
| 5 | internal/country: `ByCode` と `NamedBy` のテストを追加して実装（存在しないコード、小文字コード、国名・別名・首都名それぞれの判定） | 3 |
| 6 | internal/bot: 首都検索ヒットのテスト（東京・Tokyo→KindCapitalHit、詳細タイトルが国名、国旗画像あり。シンガポール→KindCountryHit でタイトルは入力）。実装 | 5 |
| 7 | internal/bot: 既存の国旗クイズの処理を quizFlag という名前で切り出す（タスク 9 の振り分け実装より前）。postback に code を加える。TestQuiz と TestQuizShufflesCorrectPosition を quizFlag の直接呼び出しに移し、data 形式の検証を `answer, code, result` に更新し、code が正解国と一致することを確認。期待値（文言・メッセージ数・ボタン数・正解 1 つ・位置のシャッフル）は変えない。respondAnswer を code 優先・旧形式互換にする（既存 TestAnswerCorrect / TestAnswerWrong / TestAnswerUnknownCountryStillConfirms / TestAnswerInvalidData は変更なしで通す。code 不明や不正な code の場合のテストを追加） | 5 |
| 8 | internal/bot: 首都クイズの出題テスト（文言、ラベル=capital_ja、displayText、data の形 `answer, code, q, result`、正解 1 つ、正解国の国旗、Reply.Code、位置のシャッフル）。quizCapital を実装 | 7 |
| 9 | internal/bot: 種類の 1:1 ランダム（30 シードで両種類が出る）と、首都欠け・ラベル重複時の国旗クイズへのフォールバック（Load で作ったフィクスチャ Store）。quiz の振り分けを実装 | 8 |
| 10 | internal/bot: 首都クイズの回答テスト（正解 4 通、不正解 2 通、Kind、code 不明時は確認のみ）。TestReplyCountNeverExceedsLimit に首都入力・首都 postback を追加。実装 | 7, 8 |
| 11 | scripts/local-smoke.mjs: 場面 4 を両クイズ対応にし、「クイズ」を最大 10 回送って両種類が出ることを確認する場面、首都検索（東京）の場面、旧形式 postback（code なし）の場面を追加。bench-cpu の場面に首都検索（東京）を追加 | 10 |
| 12 | docs 更新: README.md 機能表、docs/architecture.md、docs/development.md、docs/data-update.md | 10 |
| 13 | ローカル検証一式（下記の完了条件）。サイズと CPU を計測して PR 本文に記録 | 11, 12 |
| 14 | 独立レビュー（gpt-6.1-sol）。指摘を修正し再検証 | 13 |
| 15 | PR 作成、CI の必須チェック通過を確認。マージは利用者 | 14 |
| 16 | マージ後に `npx wrangler deploy`。利用者の実機確認（下記） | 15 |

## 完了条件・受け入れ基準

### テスト・静的検査（手元と CI）

- [ ] `go test -race ./...` が通る
- [ ] `go vet ./...` と `GOOS=js GOARCH=wasm go vet ./...` が通る
- [ ] gofmt の差分がない
- [ ] `go mod tidy -diff` の差分がない
- [ ] `tinygo test ./internal/...` が通る
- [ ] 鮮度テストが、再生成した countries_gen.go で通る
- [ ] CI の必須チェック go / tinygo-test / build がすべて緑

### 機能（単体テストで検証）

- [ ] 「東京」「Tokyo」で日本の詳細が返り、タイトルは「日本」、本文は「東京 / Tokyo」、Kind は KindCapitalHit
- [ ] 「シンガポール」は KindCountryHit で、タイトルは入力のまま
- [ ] 首都名が他国の国名・別名と一致するフィクスチャで、国名・別名が優先される
- [ ] 首都名どうしが重複するフィクスチャで、そのキーが索引に入らない
- [ ] 全 196 か国の capital_ja と capital_en が Lookup で自国に当たる
- [ ] 「クイズ」「quiz」「QUIZ」で、固定シードを 30 個回すと国旗クイズと首都クイズの両方が出る
- [ ] 首都クイズの Text と AltText が「この国の首都はどこでしょうか？」、ラベルが capital_ja、正解が 1 つ、正解の位置が 4 か所すべてに現れる
- [ ] 国旗クイズの文言・メッセージ数・ボタンの形は変わらない。既存の国旗クイズのテスト（TestQuiz、TestQuizShufflesCorrectPosition）は、Respond 経由ではなく quizFlag の直接呼び出しで検証する形に移し、文言・メッセージ数・ボタン数・正解 1 つなどの期待値は変えない（data の形の検証だけ code を足して更新する）。Respond 経由で「クイズ」を送ったときの種類の振り分けは、別のテスト（固定シードを 30 個回して両種類が出る）で検証する。回答側の既存テスト（TestAnswerCorrect / TestAnswerWrong / TestAnswerUnknownCountryStillConfirms / TestAnswerInvalidData）は変更なしで通る
- [ ] 首都クイズの正解で 4 通（エコー、正解、国の詳細、確認）、不正解で 2 通
- [ ] 旧形式 `answer=日本&result=1` の postback が現状と同じ返信になる
- [ ] code が不明な postback は詳細を省いて確認だけ返す
- [ ] すべての postback data が 300 文字以下、ラベルが 20 文字以下
- [ ] 返信は常に 5 通以下

### 疎通・性能

- [ ] scripts/local-smoke.mjs の全場面が通る（新規の首都検索、両クイズ、旧形式 postback を含む）
- [ ] gzip サイズが 600 KiB 以下（scripts/check-size.sh 1024 は従来どおり通る）。計測値を PR 本文に書く
- [ ] bench-cpu の「日本」「クイズ」の中央値が現状比 +10% 以内（超えたときは 3 回計測の中央値で判断）。変更前後の計測値を PR 本文に書く

### 文書

- [ ] README.md の機能表に、首都名での検索と、クイズが国名・首都のどちらかをランダムに問うことが書かれている
- [ ] docs/architecture.md の冒頭、internal/bot の役割、状態を持たない説明（postback に code と q を含む）が更新されている
- [ ] docs/development.md の local-smoke の場面一覧と bench-cpu の場面が更新されている
- [ ] docs/data-update.md に、首都名も索引に入ることと衝突時の優先順位が書かれている

### 本番

- [ ] `npx wrangler deploy` が成功し、Cloudflare の gzip サイズ表示が上限内
- [ ] 利用者の実機確認: 「東京」で日本の詳細、「シンガポール」で国として返る、「クイズ」を数回送って両種類が出る、首都クイズの正解・不正解、デプロイ前に出ていた古いクイズボタンを押しても正解・不正解が返る

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| 国旗クイズの postback に code が付く | 利用者には見えない内部変更だが、「国旗クイズを変えない」前提の例外。旧ボタンは互換経路で動く | 方針どおり実装し、PR 本文に明記 |
| ログ用 Kind の新設 | KindCapitalHit / KindCapitalQuiz / KindCapitalAnswerCorrect / KindCapitalAnswerWrong が本番ログに新しく現れる | 方針どおり実装。ログの集計に影響があれば利用者に伝える |
| 乱数消費順の変更 | 既存テストが特定の出題を固定していれば失敗する | 失敗したテストの意図を保ったまま期待値を更新する |
| local-smoke の両種類確認 | 1:1 でも 10 回送って片方しか出ない確率は約 0.2% | 失敗時は再実行で切り分ける。回数は定数にする |
| 将来のデータ更新で首都名が他国と衝突 | 索引は国名・別名優先で黙って除外し、不変条件テストが失敗して知らせる | 失敗時はデータ側の別名調整などで対応する（本 PR の範囲外） |

### 利用者が決めたこと

| 項目 | 決定 |
|---|---|
| 出題の種類 | 国旗→国名（既存）と国旗→首都の 2 種類。専用の送る言葉は足さない |
| 混ぜ方と割合 | 「クイズ」「quiz」でランダム、1:1 |
| 首都名で検索 | する。日本語（capital_ja）と英語（capital_en）の両方 |
| 首都名で見つかったときのカードのタイトル | 国名（本文は「東京 / Tokyo」） |
| 首都クイズで不正解のとき | 正解を教えない（国旗クイズと同じ 2 通） |
| 「・」「.」を除いた表記 | 今回は受け付けない（別 PR の候補） |

## 公開経路

1. 作業ブランチ plan/2026-11-01-capital-quiz に計画書と実装を commit する。
2. 独立レビュー（gpt-6.1-sol）を通し、指摘を修正する。
3. main への PR を作り、CI の必須チェック go / tinygo-test / build が通ることを確認する。PR 本文に gzip サイズと bench-cpu の計測値、国旗クイズの postback に code を足した旨、旧ボタンの互換方針を書く。
4. マージは利用者が行う。
5. マージ後に采配役が `npx wrangler deploy` で本番に出す。
6. 利用者が実機で確認する（首都検索、両クイズ、デプロイ前の古いボタン）。問題があれば直前のデプロイに戻す。
