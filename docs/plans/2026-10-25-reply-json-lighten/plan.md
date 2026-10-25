# 返信 JSON 生成の軽量化 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: 1a65280215f08dd3d511be4928020558e77fcfc6
- 作業ブランチ: plan/2026-10-25-reply-json-lighten
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 計画書の作成担当: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 編成の例外理由: なし
- 実装担当: claude-opus-5-5 / high。対象は 1 パッケージ内の限定的な書き換えだが、encoding/json と byte 単位で一致させる差分テストの設計を含むため通常段を high にする
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ実装 claude-fable-5-1 / high、レビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-reply-json-lighten/ に plan.md と request.md を commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。検収後に采配役が push・PR 作成を行い、CI 通過後のマージと本番デプロイ（npx wrangler deploy）まで利用者の承認済み。build/ 配下の生成物と一時的な TinyGo ビルドは commit しない

## 背景・目的

bot は Cloudflare Workers 上で Go の wasm として動き、workers-go はリクエストごとに新しい wasm インスタンスを main から実行する。そのため encoding/json が型ごとに reflect で組み立てる変換器はリクエストごとに作り直され、温まることがない。V8 プロファイルでは webhook 処理の包含時間のうち `encoding/json/v2.Marshal` が 27.1% を占め、その大半が変換器の組み立て（`makeStructFields` 21.2%）だった。各メッセージ型の `MarshalJSON` が alias 型を埋め込んだ匿名構造体を `json.Marshal` する作りのため、型の数だけ変換器が組み立てられ、入れ子の出力は外側で検証・再整形されている。

この PR では、返信 API とローディング API へ送るリクエスト本文の生成を、reflect を使わない手書きのエンコーダに置き換える。送る JSON の内容は変えず、現在の `encoding/json` の出力と byte 単位で一致させる。狙いは Workers 無料プランの CPU 目安 10ms に近づけること、および後続で検討している TinyGo への切り替えでも同じ効果が出ることである。

## スコープ

- 対象:
  - `internal/line/message.go`: `Message`・`Template`・`Action` の各実装型に、バッファへ直接 JSON を書き足す非公開メソッドを追加する。既存の `MarshalJSON` は残し、新メソッドへ委譲する
  - `internal/line/json.go`（新規）: 文字列エスケープ・整数・配列・オブジェクトを書き足す最小限のヘルパ
  - `internal/line/client.go`: `post` で `json.Marshal` を使わず、返信本文とローディング本文を手書きエンコーダで生成する
  - `internal/line` のテスト: encoding/json の出力との byte 一致を検証する差分テスト、文字列エスケープの表形式テストと fuzz テスト、本文生成のベンチマーク
  - 計測: Go ビルドと TinyGo の一時ビルドで `scripts/bench-cpu.mjs` によりベースと比較し、結果を完了報告に残す
- 対象外:
  - webhook 受信（`internal/line/webhook.go` の `json.Unmarshal`）の置き換え。理由は方針の節に書く
  - TinyGo の導入（ビルド手順・CI・wrangler 設定の変更）。計測のための一時ビルドのみ行い、リポジトリには残さない
  - `internal/bot`・`internal/country`・`internal/app` のロジック変更。これらは変更しない
  - メッセージ型のフィールド構成・バリデーション・LINE API の仕様に関わる変更
  - 固定メッセージ（再挑戦確認など）の JSON を事前にリテラルとして持つ最適化。効果が小さく可読性を落とす

## 方針

### 設計

1. 共通ヘルパ（`internal/line/json.go`）
   - `appendJSONString(dst []byte, s string) []byte`: 引用符つきで文字列を書き足す。エスケープ規則は現在の `encoding/json`（Go 1.27、v1 互換）と byte 単位で一致させる。規則は差分テストが正であり、実装はテストに合わせる。参考として想定する規則は次のとおり: `"` と `\` は `\"` `\\`、`\b` `\f` `\n` `\r` `\t` は 2 文字エスケープ、それ以外の 0x20 未満は小文字 16 進の `\u00XX`、`<` `>` `&` は `\u003c` `\u003e` `\u0026`、U+2028 と U+2029 は `\u2028` `\u2029`、DEL（0x7f）と有効な非 ASCII（日本語・絵文字を含む）はそのまま出力、不正な UTF-8 の byte は `\ufffd` に置換
   - 整数は `strconv.AppendInt`。オブジェクトのキーは ASCII 定数なので引用符つきの定数文字列を直接書き足してよい
   - `reflect`・`encoding/json`・`encoding/json/v2`・`jsontext` を使わない。依存は `strconv` と `unicode/utf8` に限る（TinyGo で問題なく動く範囲）

2. メッセージ型（`internal/line/message.go`）
   - `Message`・`Template`・`Action` の各インタフェースに `appendJSON(dst []byte) []byte` を追加する。既存のマーカーメソッド（`isMessage` など）は残す。マーカーを外すと 3 つのインタフェースのメソッド集合が同じになり、テンプレートをメッセージとして渡せてしまうため
   - 8 つの実装型が値レシーバで `appendJSON` を実装する。先頭に `"type":"..."` を書き、以降は構造体タグの順にフィールドを書く。`omitempty` のフィールドは空文字列なら省く。`actions` は nil スライスなら `null`、空スライスなら `[]`、入れ子の nil インタフェースは `null` を書く（いずれも現在の encoding/json と同じ）
   - `Message`・`Template`・`Action` の値が入り得るすべての位置（`[]Message` の要素、`TemplateMessage.Template`、`ButtonsTemplate.Actions` と `ConfirmTemplate.Actions` の要素）で、次の 3 種類を扱う。値レシーバのメソッドは `*T` でも呼べるため 8 型のポインタ型も各インタフェースを満たすが、型付き nil は「nil インタフェース」の判定を通り抜け、そのまま値レシーバの `appendJSON` を呼ぶと panic するため
     - nil インタフェース: ベースと同じく `null` を書く
     - 8 型それぞれのポインタ型の nil（型付き nil。例: `TemplateMessage{Template: (*ButtonsTemplate)(nil)}`）: panic せず、ベースの encoding/json と同じ出力にする（想定は `null`。実際の挙動は差分テストを正とする）
     - 8 型それぞれの nil でないポインタ: 指す値と同じ出力にする（ベースの encoding/json と同じ）
   - 上の判定は `reflect` を使わず、型スイッチ（各インタフェースの値を 8 型のポインタ型で場合分けする等）で行う。`appendJSON` は値レシーバのままでよい
   - 各型の `MarshalJSON` は残し、`appendJSON(nil)` の結果を返す。`json.Marshal(msg)` を使う既存コード（テスト・デバッグ用途）の互換を保つ。`message.go` から `encoding/json` の import は消える
   - `MarshalJSON` を直接呼ぶ場合の挙動（型付き nil で値レシーバのメソッドを呼ぶと panic する Go の規則）はベースと同じで変えない。`json.Marshal` 経由の型付き nil は encoding/json 自身が `null` を書くので、従来どおりになる
   - 出力はコンパクト（空白なし）。キーの順序は現在と同じ（type が先頭、以降はフィールド宣言順）。順序と空白を変えない理由は、byte 一致で「内容が変わっていない」ことを機械的に示せるようにするため

3. クライアント（`internal/line/client.go`）
   - `post(ctx, api, path string, body []byte) error` に変更し、`Reply` は `{"replyToken":"...","messages":[...]}` を、`ShowLoading` は `{"chatId":"...","loadingSeconds":N}` を手書きエンコーダで生成して渡す。`replyRequest`・`loadingRequest` 構造体は削除するか、本文生成関数（例: `appendReplyBody`・`appendLoadingBody`）に置き換える
   - バッファは 1 リクエストにつき 1 本を作って使い回す。初期容量は返信本文の典型サイズ（数百 byte から 2KB 程度）を目安に実装担当が決める
   - エンコード失敗の経路（`fmt.Errorf("line: encode ...")`）は手書きでは起こらないので消える。既存テストに依存する箇所がないことを確認する

### 不変条件・契約

- 任意の `Message` 値（入れ子に値・nil でないポインタ・nil インタフェース・型付き nil を含むものを含む）について、新エンコーダの出力は、ベース SHA の方式（alias 埋め込み構造体の `json.Marshal`）で得る出力と byte 単位で一致し、panic しない。この「参照エンコーダ」はテストファイル内に、本体の型を使わない写しの型で持つ（後述）
- 任意の Go 文字列について `appendJSONString` の出力は `json.Marshal(s)` と byte 単位で一致する
- `Reply` と `ShowLoading` が送る本文は、ベース SHA が送る本文と byte 単位で一致する
- 既存のゴールデンテスト（`internal/line/testdata/messages/*.json`、構造比較）と `internal/app` のテストは、ゴールデンファイルを変更せずに通る
- 公開 API（型名・フィールド・`Reply`・`ShowLoading`・`APIError`・`ErrSend`・エラーメッセージの文面）は変えない

### 代替案と選ばなかった理由

- `MarshalJSONTo(*jsontext.Encoder)`（json/v2 の MarshalerTo）を実装する: 入れ子出力の再検証は避けられるが、最上位での変換器組み立てと reflect は残る。json/v2 公開 API への依存が生じ、TinyGo での可否が不明。採らない
- easyjson などのコード生成: 依存とツール導入が増える。対象は 8 型のみで手書きの方が小さく、TinyGo 互換も確実。採らない
- 固定メッセージを JSON リテラルで事前に持つ: 手書きエンコーダはマイクロ秒単位なので追加の効果が小さい。採らない
- webhook 受信の `json.Unmarshal` も置き換える: 包含 6.1% と効果が小さい一方、外部入力のパーサを自作するリスクが大きい（未知フィールドの読み飛ばし、エスケープ解釈、数値、深い入れ子）。別 PR で fuzz を伴って検討する。なお `encoding/json` は受信側で引き続きリンクされるため、この PR では wasm サイズの大幅な減少は期待しない

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | テスト用の参照エンコーダを `internal/line` のテストファイルに置く。本体の型を使わない「写しの型」（ベース SHA の各型と同じフィールド・同じ json タグを持ち、先頭に `type` フィールドを持つ構造体）を定義し、本体の値をこの写しの型へ再帰的に変換してから `encoding/json` で `Marshal` する。変換は `Message`・`Template`・`Action` の動的型（値、ポインタ、nil インタフェース、型付き nil）を型スイッチで網羅し、入れ子（`Template`、`Actions` の要素、`[]Message` の要素、返信本文・ローディング本文の外側のオブジェクト）も写しの型に置き換える。期待値の生成で本体の `MarshalJSON`・`appendJSON` を一切呼ばず、未知の動的型はテスト失敗にする。変換が本体を呼ばないことをテストで担保する（方法は実装担当が選ぶ。例: 写しの型が本体のインタフェースを満たさないことをコンパイル時に確かめる、変換結果に本体の型が残っていないことを確かめる）。参照エンコーダの出力がベースと同じであることの錨として、既存のゴールデンファイル（`internal/line/testdata/messages/*.json`）に対する構造比較を参照エンコーダの出力にも行う（ゴールデンファイルは変更しない）。本体コードは変えない | - |
| 2 | 文字列エスケープのテストを先に書く。表形式（空文字、ASCII、日本語、絵文字とサロゲートペア、`"` `\`、全制御文字 0x00〜0x1f、DEL、`<>&`、U+2028/U+2029、不正 UTF-8 byte 列、5000 文字の長文）で `json.Marshal(s)` との byte 一致を検証し、`FuzzAppendJSONString` を追加する。失敗を確認してから `json.go` に `appendJSONString` を実装する | 1 |
| 3 | 8 型の `appendJSON` のテストを書く。既存ゴールデン 5 件と actions 1 件、omitempty の全組み合わせ、nil Template・nil Action・nil と空の Actions、1〜5 件の `[]Message`、`[]Message` の要素・`TemplateMessage.Template`・`Actions` の要素の各位置での nil インタフェース・型付き nil・nil でないポインタの 3 種類について参照エンコーダとの byte 一致を検証する（型付き nil で panic しないことも確かめる）。失敗を確認してからインタフェースと 8 型の `appendJSON` を実装し、`MarshalJSON` を委譲に変える。既存の `assertGolden` はそのまま残す | 2 |
| 4 | `client_test.go` に返信本文・ローディング本文の byte 一致テストを追加し、失敗を確認してから `post` と本文生成を書き換える。`message.go` と `client.go` から `encoding/json` の import を外す | 3 |
| 5 | `internal/line` に本文生成のベンチマーク（国名回答相当とクイズ相当の 2 本、`ReportAllocs`）を追加する。`internal/app` の既存ベンチをベースと比較して記録する | 4 |
| 6 | ローカルで CI と同じ検査を通す: gofmt、`go mod tidy -diff`、`go vet ./...`、`go test -race ./...`、`GOOS=js GOARCH=wasm go vet ./...`、`npm run build`、`scripts/check-size.sh`。fuzz は短時間（例: 30 秒）ローカルで回し、見つかった差分は表形式テストに追加して直す | 4 |
| 7 | Go ビルドでベースと PR を `scripts/bench-cpu.mjs` で比較する（手順は受け入れ基準）。続いて TinyGo の一時ビルドで同じ比較を行う。終了後に `npm run build` で build/ を Go 版に戻し、生成物を commit しないことを確認する | 6 |
| 8 | `message.go`・`json.go` の先頭コメントに、エンコーダを手書きにした理由（毎リクエストの reflect 回避）と、encoding/json との byte 一致を差分テストで保証していることを書く。commit を作る | 7 |

## 完了条件・受け入れ基準

送る JSON が変わらないこと
- [ ] `internal/line` の差分テストが通る: 8 型すべて、`[]Message`、返信本文、ローディング本文について、新エンコーダの出力がテスト内の参照エンコーダ（本体の型を使わない写しの型による、ベース SHA の方式）と byte 単位で一致する。項目には、`[]Message` の要素・`TemplateMessage.Template`・`Actions` の要素の各位置での nil インタフェース・型付き nil・nil でないポインタの 3 種類を含み、返信本文（`Reply` が送る本文）にも型付き nil を含むケースを 1 つ以上含む。型付き nil を含む値で panic しない
- [ ] 参照エンコーダが期待値の生成で本体の `MarshalJSON`・`appendJSON` を呼ばないことがテストで担保されている
- [ ] 参照エンコーダの出力が既存のゴールデンファイル（`internal/line/testdata/messages/*.json`）と構造比較で一致する（ゴールデンファイルは変更しない）
- [ ] 文字列エスケープの表形式テストが通り、`go test -run '^$' -fuzz FuzzAppendJSONString -fuzztime 30s ./internal/line` を実装担当がローカルで実行して失敗が出ない（CI では seed のみ実行される）
- [ ] 既存のゴールデンテスト（`testdata/messages/*.json`）がファイル変更なしで通る
- [ ] `internal/app` の既存テストが変更なしで通る（ハンドラ経由の本文が従来どおりであることの確認）
- [ ] `json.Marshal(msg)` が従来と同じ出力を返す（`MarshalJSON` を残して委譲している）

コード品質・CI
- [ ] `gofmt -l .` が空、`go mod tidy -diff` が差分なし、`go vet ./...` と `GOOS=js GOARCH=wasm go vet ./...` が通る
- [ ] `go test -race ./...` が通る
- [ ] `npm run build` が成功し、`scripts/check-size.sh` が通る（gzip 2.5MiB 上限）。ベースと PR の gzip サイズを完了報告に記録する。増加していれば理由を書く。減少は期待するが条件にしない
- [ ] `internal/line/message.go` と `internal/line/client.go` から `encoding/json` の import が消えている（`internal/line/webhook.go` は残る）
- [ ] PR 作成後、.github/workflows/ci.yml の test ジョブが通る

計測（Go ビルド、`scripts/bench-cpu.mjs`）
- [ ] 手順: ベース SHA と PR をそれぞれ `npm run build` し、`build/app.wasm` をリポジトリ外の作業用ディレクトリに別名で保存する。同じマシン・同じセッションで、ベースと PR を交互に 3 回ずつ `node scripts/bench-cpu.mjs <wasm> 30` を実行する（各回は 30 回実行の後半 28 回の中央値）。計測中は他の重い処理を動かさない
- [ ] 合格ライン: 「webhook 日本」「webhook クイズ」の両シナリオで、PR の handle の 3 回の中央値がベースの handle の 3 回の中央値より小さい。handle にはモックの返信 API へ送る Node の fetch の CPU が含まれて半分以上を占め、本変更では減らないため、Go 側の削減（数 ms）は handle 全体に対する比率として小さく出る。そこで比率（10%）や分布の分離ではなく減少の向きで判定する
- [ ] 回帰なし: 「GET /」の start と total、各 webhook シナリオの start について、PR の 3 回の中央値が「ベースの 3 回の最大値 + 0.5ms」を超えない（初期化に仕事を足していないことの確認。0.5ms は start の回ごとのぶれに対する許容幅）
- [ ] 結果（6 回分の生の表と判定）を完了報告に残す

計測（TinyGo 0.43.0 の一時ビルド）
- [ ] 手順: Go 版の計測をすべて終えた後に行う。`go run github.com/syumai/workers-go/cmd/workers-assets-gen -mode=tinygo` と `tinygo build -o ./build/app.wasm -target wasm -no-debug ./cmd/worker` をベース SHA と PR のそれぞれで実行し、wasm を別名で保存する。Go 版と同じ交互 3 回・runs=30 で計測する。終了後に `npm run build` で build/ を Go 版に戻す
- [ ] 合格ライン: Go 版と同じ（webhook 2 シナリオで PR の handle の中央値がベースの中央値より小さい。start と GET / は、PR の中央値がベースの最大値 + 0.5ms を超えない）。TinyGo 版の計測も合格条件とする
- [ ] PR の変更が TinyGo でコンパイルできる（`tinygo build` 成功）。`tinygo test ./internal/line` が実行できれば実行し結果を記録する。ネットワーク系の依存（net/http など）で TinyGo 側の制約により実行できない場合は、その旨と出力を完了報告に書く。これは合格条件にしない
- [ ] 一時ビルドで生じた build/ の変更やその他のファイル変更が commit に含まれていない

ホストのベンチマーク
- [ ] `go test -run '^$' -bench Webhook -benchmem -count 6 ./internal/app` をベースと PR で実行し、Country と Quiz の両方で allocs/op が減っている。ns/op も中央値で減っている（ホストは温まった状態なので差は wasm より小さい。allocs/op を主指標とする）
- [ ] `internal/line` の新ベンチマークが実行でき、結果を完了報告に残す

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| ローディング API 本文の対象化 | 確定: 返信本文と一緒に手書きにする。同じ webhook 内で返信の直前に送られ、型が 1 つ増えるだけで `client.go` から encoding/json を完全に外せる | 対象に含める |
| webhook 受信（Unmarshal）の対象化 | 確定: 対象外。包含 6.1% に対して外部入力パーサのリスクが大きい。必要なら別 PR で fuzz を伴って検討する | 対象外。実装担当は触らない |
| エスケープ規則の細部 | 方針に書いた規則は想定であり、Go 1.27 の実際の挙動（特に `\b` `\f`、不正 UTF-8 の `\ufffd` 置換）は差分テストで確認する | テストが正。想定と異なる挙動が見つかれば実装をテストに合わせ、完了報告に差異を書く。LINE が受け付けない形になり得る場合は止まって報告 |
| 不正 UTF-8 の扱い | encoding/json は `\ufffd` に置換する。入力は LINE から来る有効な UTF-8 だが、国名データや環境変数の値が経路になり得る | encoding/json と同じ置換で実装する。エラーにはしない |
| 参照エンコーダの持ち方 | 確定: テストファイル内に本体の型を使わない写しの型（ベース SHA の各型と同じフィールド・同じ json タグ、先頭に `type` フィールド）を定義し、本体の値を型スイッチで写しの型へ再帰的に変換してから `encoding/json` で `Marshal` する。ベースの alias 埋め込み方式をそのまま写すと、`Template`・`Actions`・`[]Message` の要素の動的型が本体の型のままで、委譲後は入れ子で新エンコーダを呼んでしまい独立した比較にならないため。代替: ゴールデンファイルから byte 列を作る（空白除去が要り、ランダム入力に使えない） | 写しの型で進める。期待値の生成で本体の `MarshalJSON`・`appendJSON` を呼ばないことをテストで担保し、参照エンコーダの出力もゴールデンファイルと構造比較する。未知の動的型はテスト失敗にする。参照エンコーダは本体コードから参照しない |
| マーカーメソッドの扱い | `isMessage` などを残して `appendJSON` を追加する。外すと 3 インタフェースが同一の集合になり型の区別が消える | 残す。実装担当が別案を採る場合は理由を commit メッセージに書く |
| バッファ初期容量 | 典型本文は数百 byte から 2KB 程度。過大だと無駄、過小だと再割り当て | 実装担当が `internal/line` のベンチの allocs/op を見て決める |
| TinyGo でのテスト実行 | `tinygo test` が `net/http` 依存で動かない可能性がある | ビルド成功を必須、テスト実行は可能なら。できない場合は出力を添えて報告 |
| build/ の追跡状況 | `.gitignore` が `/build/` を除外しており、build/ は追跡されていない | 生成物が diff に出ないことを確認する |
| 計測のぶれ | 数 ms のぶれがあり、Node の fetch が handle の半分以上を占めるため、Go 側の削減は handle 全体に対する比率として小さく出る | 交互 3 回の中央値で減少の向きを判定し、start は 0.5ms の許容幅で回帰を判定する。基準に達しない場合は、生データを添えて止まって報告する |
| CI の fuzz | CI では seed のみが走る。生成コーパスの commit 可否 | 生成コーパス（testdata/fuzz/）は commit しない。見つかった反例は表形式テストに固定する |
| 本番での効果 | 本番の CPU 中央値 37ms には起動直後の重さが含まれ、Node の計測と絶対値が異なる | 本 PR の受け入れは Node 計測で判定する。本番値はデプロイ後に利用者が wrangler tail で記録し、別途評価する |

## 公開経路

1. 実装担当が作業ブランチ plan/2026-10-25-reply-json-lighten にテスト・実装・ドキュメント（plan.md、request.md、コメント）を commit する。push はしない
2. 実装レビュー担当（read-only）が差分と計測結果を検収する。通常段で 3 回合格しなければ昇格編成に切り替える
3. 検収後に采配役が push し、main への PR を 1 本作る。CI（gofmt、tidy、vet、test -race、js/wasm vet、build、check-size）の通過を確認してマージする
4. 本番デプロイ（wrangler deploy）は CI 通過・マージ後に行う（利用者の承認済み）。デプロイ後は利用者が実機で返信（国名・クイズ・回答・非テキスト）を確認し、wrangler tail の CPU 時間・応答時間を記録する
5. TinyGo への切り替えは別 PR として、この PR の計測結果を参考に判断する
