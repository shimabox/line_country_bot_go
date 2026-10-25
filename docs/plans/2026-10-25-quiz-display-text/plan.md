# クイズ選択肢の displayText 表示 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: a41c636258d69090e4d5f484d656296f40d556cc
- 作業ブランチ: plan/2026-10-25-quiz-display-text
- 起案担当: 予定: claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画書の作成担当: 予定: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 計画レビュー担当: 予定: gpt-6-astra / high（未実行）
- 編成の例外理由: なし
- 実装担当: claude-opus-5-5 / high。理由: 通常段の編成（計画承認時に確定）
- 実装レビュー担当: 予定: gpt-6.1-sol / high
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-quiz-display-text/ へ commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。push・PR 作成・CI 成功後のマージ・本番デプロイは検収後に采配役が利用者の承認範囲で行う

## 背景・目的

国旗の 4 択クイズで選択肢を押しても、トーク上には何も表示されず、自分がどれを押したのか分からない。LINE では送信済みのカードを書き換えられないため、押した結果は新しい表示で示すしかない。

この変更では、ポストバックアクションの displayText に選択肢の国名を入れる。押すと、その国名が利用者の発言としてトークに表示され、どれを押したかが残る。はずれたら同じカードの別の選択肢を押し直せる現在の遊び方は変えない。

## スコープ

- 対象:
  - `internal/line/message.go`: PostbackAction に displayText フィールドと上限定数を追加
  - `internal/bot/bot.go`: クイズの選択肢に displayText として国名を設定
  - `internal/line/message_test.go` と `internal/line/testdata/messages/`: JSON 出力の golden テスト追記
  - `internal/bot/bot_test.go`: クイズの選択肢に displayText が入ることの検証
  - `scripts/local-smoke.mjs`: クイズ場面（4）で displayText の存在を検査
- 対象外:
  - 答えの判定（postback data の形式、`respondAnswer` の処理）の変更
  - 結果カードや選択肢を減らしたカードなど新しいメッセージの追加
  - 返答のリズム（入力中アニメーション・待ち時間）。別の PR で扱う
  - 返答文言の変更（正解・はずれ時の echo を含む）

## 方針

- **displayText を使い、廃止予定の text は使わない。** Messaging API ではポストバックアクションの `text` が廃止予定で、`displayText` が後継。両者は同時指定不可だが、PostbackAction に text フィールドは無いので衝突しない。
- **PostbackAction に `DisplayText string` を `json:"displayText,omitempty"` で追加する。** 空のときはキーを出力しないので、クイズ以外で使う既存のポストバック出力と既存の golden ファイルは変わらない。これが後方互換の不変条件。
- **上限定数 `MaxPostbackDisplayText = 300` を既存の上限定数群に並べる。** 単位は他と同じ UTF-16 コードユニット。
- **displayText には国名の全文（`c.NameJA`）を入れる。** ラベルは 20 ユニットに切り詰められるが、displayText の上限は 300 で、同じ国名はすでに postback data（上限 300）に全文で入っているため、切り詰めなしで収まる。テストで上限内であることを検証する（計画承認時に確定）。
- **判定経路は一切触らない。** displayText は Webhook に送られず表示専用なので、`respondAnswer` と postback data の形式に影響しない。

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | `internal/line`: golden テスト追記（red）。`testdata/messages/actions.json` に displayText 付きポストバック要素を 1 つ追加し、`TestActionGolden` にも対応する PostbackAction を追加。既存要素は変更しない | - |
| 2 | `internal/line`: PostbackAction に `DisplayText` フィールド（`displayText,omitempty`）と `MaxPostbackDisplayText = 300` を追加し、1 を green にする。既存 golden（buttons_no_image.json など）が無変更で通ることを確認 | 1 |
| 3 | `internal/bot`: `TestQuiz` に検証を追記（red）。各アクションで `DisplayText` が postback data の `answer` 値と一致し、`UTF16Len` が `MaxPostbackDisplayText` 以下であること | 2 |
| 4 | `internal/bot`: `quiz` の PostbackAction 生成に `DisplayText: c.NameJA` を追加し、3 を green にする | 3 |
| 5 | `scripts/local-smoke.mjs`: 場面 4 の判定に、全アクションの `displayText` が data の `answer` と一致する条件を追加 | 4 |
| 6 | `gofmt` / `go vet` / `go test ./...` を実行し、結果を記録して commit | 5 |

## 完了条件・受け入れ基準

- [ ] `go test ./...` が成功する
- [ ] `go vet ./...` が成功し、`gofmt -l` の出力が空
- [ ] 次の検証がすべて成功する: `gofmt -l .` の出力が空、`go vet ./...`、`go test -race ./...`、`GOOS=js GOARCH=wasm go vet ./...`、`go mod tidy -diff` が差分なし、`npm ci` と `npm run build`、`bash scripts/check-size.sh`
- [ ] `TestActionGolden` で displayText 付きポストバックが `{"type":"postback","label":...,"data":...,"displayText":...}` として出力される
- [ ] 既存 golden ファイル（`buttons_no_image.json`、`actions.json` の既存要素）は変更なしで通り、displayText 未設定のときキーが出力されない
- [ ] `TestQuiz` で、全 30 シード × 3 入力において 4 つの選択肢すべてで `DisplayText` が data の `answer` と等しく、長さが 300 UTF-16 ユニット以下
- [ ] `TestQuizShufflesCorrectPosition` と `respondAnswer` 系のテストが無変更で通る（判定経路に変更がない証拠）
- [ ] `scripts/local-smoke.mjs` の場面 4 が displayText を検査するよう更新され、`node scripts/local-smoke.mjs`（wrangler dev と返信 API のモックを使う既存の疎通確認）の既存の全場面（場面 1〜8 と `GET /flags/jp.png`）が成功し、場面 4（クイズ）で 4 つの選択肢すべての displayText が data の answer と一致する。実装担当の環境で wrangler dev を起動できない場合は、その旨を報告し、采配役が同じ確認を実行する
- [ ] リポジトリ既存の CI（Go のテスト系 check）が PR で成功する。PR 作成は検収後のため、実装担当はローカル結果を報告する
- [ ] デプロイ後の実機確認（采配役と利用者）: クイズで選択肢を押すと、押した国名が自分の発言としてトークに表示され、続けて従来どおり bot の返答が届く。はずれ後に同じカードの別の選択肢を押し直せる

## 未確定事項・リスクと判断の委ね方

計画承認時に確定した事項:

- displayText には国名の全文（`c.NameJA`）を入れる
- 押したあとの「[国名] ですね?」の聞き返しは変えない
- 実装担当は claude-opus-5-5 / high

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| golden の置き場 | 新要素を `actions.json` に追記するか、専用ファイルを作るか | 実装担当の判断でよい。既存要素を変更しないこと |
| 国名が 300 ユニット超 | 理論上の懸念のみ。data の上限テストが既に 300 を保証している | テストで検出できるので追加の切り詰めは不要。もし失敗したら止まって報告 |
| LINE クライアントの挙動 | displayText の実表示はデプロイ後の実機でしか確認できない | 実機確認は検収後の工程。実装担当は API 仕様どおりの JSON 出力をテストで示せば完了 |
| echo の重複感 | 押した国名が利用者側に出た直後、bot が「[国名] ですね?」と返すので同じ名前が 2 回並ぶ | 本 PR では文言を変えない（計画承認時に確定）。必要なら返答のリズムの PR で検討する |

## 公開経路

1. 作業ブランチ plan/2026-10-25-quiz-display-text に計画書と実装を commit（実装担当はここまで）
2. 検収後、采配役が push と main への PR 作成を行う（利用者の承認範囲内）
3. CI 成功後にマージ。マージ後の本番デプロイは既存の運用に従い、采配役が利用者の承認を得て実行
4. デプロイ後に利用者が実機で、クイズの選択肢を押して国名が表示されることを確認

2 以降は実装担当の範囲外。
