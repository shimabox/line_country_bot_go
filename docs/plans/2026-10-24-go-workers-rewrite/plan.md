# 国名 bot の Go + Cloudflare Workers 移植 実装計画

- 日付: 2026-10-24
- リポジトリ: github.com/shimabox/line_country_bot_go（GitHub remote は未作成。ローカルの line_country_bot_go）
- ベースブランチ: main
- ベース SHA: 11fad443c5d3950d75826c7ddb5b6b9dc2819cf2
- 作業ブランチ: plan/2026-10-24-go-workers-rewrite
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認（結果に実値なし）
- 計画書の作成担当: 予定 claude-opus-5-5 / medium（起案と同じプロバイダー）
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 編成の例外理由: なし
- 実装担当: claude-opus-5-5 / high。理由: TDD で Go パッケージ設計と Wasm ビルド検証を一貫して行うため、通常段の編成を使う。下位モデルへの委譲はしない
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ、実装 claude-fable-5-1 / high とレビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-24-go-workers-rewrite/plan.md と request.md として commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。GitHub リポジトリ作成・push・PR、Cloudflare へのデプロイ、シークレット登録、LINE チャネル作成と Webhook URL 設定は利用者の承認または利用者自身の操作が必要

## 背景・目的

元の bot は PHP/Lumen 製で、PHP 実行環境と SSL サーバーの維持、手作業で更新する CSV（SJIS）が必要だった。本計画では同じ振る舞いを Go で書き直し、Cloudflare Workers（Go→WebAssembly、無料プラン）で動かす。国データは Wikidata（CC0）から再生成でき、国旗画像は Flagpedia（パブリックドメイン）から取得して Workers の Static Assets で自前配信する。これにより、サーバー運用なし・外部 SDK なし・データ更新はスクリプト再実行だけ、という状態にする。

利用者から見える機能は元 bot と同一にする。国名を送ると国旗と首都、Wikipedia と Google Maps へのボタンが返り、「クイズ」で国旗 4 択クイズが出て、回答後に正誤と再挑戦の確認が返る。

## スコープ

- 対象:
  - Go モジュール一式（署名検証、Webhook 解析、返信 API クライアント、メッセージ型、国データ検索、bot 本体、Workers エントリ）
  - 国データ生成スクリプト（Wikidata SPARQL → JSON、手動補正ファイル付き）と生成物の同梱
  - 国旗ダウンロードスクリプトと画像（public/flags/*.png）の同梱
  - wrangler 設定、npm scripts、ローカル疎通手順、README、ライセンス表記（コード MIT、データ出典）
  - GitHub Actions の CI 定義（push 後に動く。今回はファイル追加のみ）
- 対象外:
  - Flex Message への変更、リッチメニュー、クイックリプライなど新 UI
  - Cloudflare Containers、KV/D1 などのストレージ（状態を持たない）
  - デプロイ、シークレット登録、LINE チャネル作成、Webhook URL 設定（利用者操作）
  - 元 bot の CSV（アマノ技研）の再利用。データソースは Wikidata に置き換える

## 方針

### 全体構成

```
.
├── cmd/worker/main.go            // js && wasm 専用。workers.Serve と fetch アダプタだけ
├── internal/line/                // LINE プロトコル最小実装（js 非依存）
│   ├── signature.go              // HMAC-SHA256 + Base64 検証
│   ├── webhook.go                // Webhook リクエスト/イベント型と解析
│   ├── message.go                // text / sticker / template(buttons, confirm) / actions
│   └── client.go                 // 返信 API クライアント（Doer インターフェース経由）
├── internal/country/             // 国データモデル・検索・URL 生成。countries.json を embed
│   └── data/countries.json
├── internal/bot/                 // 入力正規化、トリガー語、絵文字、各返信の組み立て
├── internal/app/                 // http.Handler（ルーティング、署名検証、イベント分配、返信送信）
├── tools/gencountries/           // Wikidata から countries.json を生成（ホスト実行）
│   ├── query.sparql
│   └── overrides.json            // 表示名・首都・除外の手動補正
├── tools/fetchflags/             // flagcdn から PNG を取得（ホスト実行）
├── public/flags/*.png            // Static Assets
├── wrangler.jsonc / package.json / go.mod / README.md / LICENSE / NOTICE / .github/workflows/ci.yml
```

- internal/ 配下はすべて `GOOS=darwin` で `go test ./...` できる。`cmd/worker` のみ `//go:build js && wasm`。ワイルドカード指定ではビルド制約で除外されたディレクトリはスキップされるので、ホストの `go test ./...` は失敗しない。
- 外部 I/O は 2 か所だけ。LINE 返信 API の HTTP 送信（`Doer` インターフェース）と環境変数読み出し（`func(string) string`）。両方を `cmd/worker` で workers-go の実装に差し替える。
- Go 1.27.3 を使う。`go.mod` の `go` ディレクティブを `1.27.3` にすると、開発機の go1.27.1 は `GOTOOLCHAIN=auto`（既定）で 1.27.3 を自動取得して切り替える。CI は `actions/setup-go` に `go-version-file: go.mod` を渡す。
- 依存は `github.com/syumai/workers-go v0.36.0` のみ。LINE SDK は使わない。

### LINE プロトコル最小実装（internal/line）

- 署名検証: `x-line-signature` を Base64 デコードし、チャネルシークレットを鍵にした HMAC-SHA256(生ボディ) と `hmac.Equal` で比較する。ボディは JSON 解析前の生バイト列を使う。ヘッダー欠落・Base64 不正・不一致はすべて「無効」。
- Webhook 解析: `destination` と `events` を持つ構造体。イベントは `type`, `replyToken`, `timestamp`, `source`, `deliveryContext.isRedelivery`, `message{id,type,text}`, `postback{data}` を保持する。未知の `type` や `message.type` はエラーにせず保持し、上位層で無視する。`events: []` は正常（疎通確認）。
- メッセージ型: `TextMessage`, `StickerMessage`, `TemplateMessage{altText, template}`、`ButtonsTemplate{thumbnailImageUrl?, imageAspectRatio?, imageSize?, imageBackgroundColor?, title?, text, actions}`、`ConfirmTemplate{text, actions}`、アクションは `postback{label,data}`, `message{label,text}`, `uri{label,uri}`。`json.Marshal` で LINE の JSON と一致することをゴールデンテストで固定する。
- 返信クライアント: `POST {LINE_API_BASE_URL}/v2/bot/message/reply`、`Content-Type: application/json`、`Authorization: Bearer <token>`。メッセージは最大 5 件で、超過はクライアント側でエラーにする。2xx 以外はステータスとボディ先頭（最大 512 バイト）を含むエラーを返す。送信は `Doer interface{ Do(*http.Request) (*http.Response, error) }` に委ねる。
- 制約の内蔵: ボタンテンプレートの title は 40 文字、画像あり text は 60 文字、ラベルは 20 文字、postback data は 300 文字。文字数は UTF-16 コードユニットで数える。`internal/line` に `TruncateUTF16(s, max)` を置き、上位層が使う。

### 国データ（internal/country と tools/gencountries）

- 生成元: Wikidata SPARQL。条件は確認済みクエリに合わせる（P31=Q3624078、P297 あり、P576 なし）。取得項目は国の ja/en ラベル、ja の `skos:altLabel`、P1448 公式名称（ja）、ja.wikipedia の記事 URL と記事タイトル、首都 P36（P582 なし）の ja/en ラベル・P625 座標・QID、国自体の P625。1 国が複数行になるので Go 側で ISO コードごとに集約する。User-Agent に連絡先入りの識別子を付ける（Wikidata の利用ポリシー）。
- 1 国 1 レコードに正規化する規則:
  - `code`: ISO alpha-2 大文字。重複は生成失敗。
  - `name_ja`（表示名、クイズのボタンラベル、テンプレ title、Wikipedia URL に使う）: ja.wikipedia の記事タイトルを採用する（例: 「オランダ」「イギリス」「アメリカ合衆国」）。記事がない国は ja ラベル。overrides.json で上書き可。20 文字（UTF-16）を超えたら overrides が無い限り生成失敗。
  - `aliases_ja`: ja ラベル、altLabel、P1448、overrides の追加語。`aliases_en`: en ラベル。検索は `name_ja`・`aliases_ja`・`aliases_en` に対する完全一致（正規化後、英語は小文字化比較）。元 bot の「正式名と通称の完全一致」を包含する。
  - 首都: P36 が 1 件ならそれを採用。複数件は overrides.json で採用する首都の QID を指定し、未指定なら生成失敗（確認済みの ZA, BO, IL, PS, NL などを初回に登録する）。0 件は overrides で指定するか、空のまま許容する（テンプレ text は「首都情報なし」、地図は国の座標）。
  - 首都 ja 名の末尾「都」「市」「特別市」は機械的に除く（東京都→東京、平壌市→平壌、ソウル特別市→ソウル）。overrides が最終決定。
  - `lat`/`lon`: 首都の P625。無ければ国の P625。
  - `wikipedia_ja_url`: sitelink の URL をそのまま使う（パーセントエンコード済み）。無ければ `https://ja.wikipedia.org/wiki/` + `url.PathEscape(name_ja)`。
  - overrides.json は `exclude`（コードの配列）も持ち、含めたくない行を外せる。既定では SPARQL の結果（196 件、TW/KP/PS/XK 含む）をそのまま含める。
- 出力: `internal/country/data/countries.json`。コード昇順、インデント付き、先頭に `generated_at`（日付）と `source` を持つ。`go:embed` で読み込み、起動時に `map[正規化名]*Country` の索引を作る。
- URL 生成（Country のメソッド）:
  - 国旗: `{baseURL}/flags/{小文字コード}.png`
  - 地図: `https://www.google.com/maps/search/?api=1&query={lat},{lon}`（Google Maps URLs の公式形式。元 bot の maps.google.co.jp 形式より安定）
  - Wikipedia: 上記 `wikipedia_ja_url`
- 乱数は `*rand.Rand`（math/rand/v2）を注入し、テストは固定シードで決定的にする。`PickN(n)` は重複なしで n 件返す。件数不足時は全件を返す。

### 国旗画像（tools/fetchflags）

- countries.json の全コードについて `https://flagcdn.com/w320/{小文字コード}.png` を `public/flags/` に保存する。既存ファイルはスキップし、`--force` で再取得。取得失敗はコードを列挙して非 0 終了。
- 画像は Static Assets として配信する。wrangler の `assets.directory` を `./public` にする。パスが一致すれば Worker より先に配信され、Worker 実行回数に数えない。`public/` には配信してよいものだけを置く。
- 出典表記: README と NOTICE に Flagpedia（flagcdn.com、パブリックドメイン）と Wikidata（CC0）を記す。

### bot の振る舞い（internal/bot、元 bot と同一）

入力正規化: 半角・全角スペース、改行、タブなど `unicode.IsSpace` と U+3000 をすべて除去し、ASCII を小文字化した `lower` と、空白除去のみの `text` を作る。

| 条件（上から順に判定） | 返信（順序どおり、最大 5 件） |
|---|---|
| message でも postback でもないイベント（follow, join, unsend など） | 返信なし、200 |
| message で `message.type != text` | text「テキストメッセージ以外わからないです」、sticker(packageId "446", stickerId "1999") |
| `lower` が「君の名は」「who」で、プロフィール設定あり | buttons(title=PROFILE_TITLE, text=PROFILE_TEXT, 画像=PROFILE_IMAGE_URL, uri アクション(PROFILE_LINK_TEXT→PROFILE_LINK_URL)) |
| `lower` が「クイズ」「quiz」 | クイズ（下記） |
| `text` が「いいえ」 | 返信なし、200 |
| 上記以外（国名として検索） | 見つかった: text「[入力] ですね🤔」、text「わかります{笑顔系}」、buttons(title=入力, text="首都ja / 首都en", 画像=国旗, uri Wikipedia, uri Google Maps)。見つからない: text「[入力] ですね🤔」、text「む、むむっ、、わかりませんでした{悲しみ系}」 |
| postback `answer=<名>&result=0\|1` | text「[名] ですね?」、text「む、むむっ、、\n正解です{祝福系}」または「む、むむっ、、\n残念！はずれです{残念系}」。正解時はさらに buttons(title=名, text=首都, 画像なし, Wikipedia/Google Maps) と confirm「もう一度やりますか？」(message「いいえ」→"いいえ", message「はい」→"クイズ") |

- クイズ: 4 国を重複なしで選び、1 国を正解にする。buttons(title「クイズです」, text「この国はどこでしょうか？」, 画像=正解の国旗, postback×4)。postback data は `url.Values{"answer": name_ja, "result": "0"/"1"}.Encode()`。アクション順はシャッフルする。altText は text と同じ。
- 国旗サムネイルは `imageAspectRatio: rectangle` に `imageSize: contain`、`imageBackgroundColor: #FFFFFF` を付け、横長・正方形・ネパール形の旗が切れないようにする（元 bot の既定 cover からの意図的な変更。旗の全体を見せるため）。
- 絵文字は Go の Unicode リテラルで持つ。グループと内容は下記「返信文言と絵文字」の表のとおり。
- 元 bot の `\r` 改行は `\n` にする。
- 防御: テンプレ title は 40 文字、text は 60 文字に切り詰める。正解 postback の国名が索引に無い（データ更新で名前が変わった等）場合は詳細テンプレだけ省き、確認テンプレは出す。postback data の解析失敗や `answer` 欠落は返信なしでログ出力。
- プロフィール設定は `PROFILE_IMAGE_URL` が空なら無効（元 bot の PROFILE_IMG に対応）。有効なら他 4 項目の必須検査を起動時に行い、欠けていれば自己紹介を無効にして警告ログ。
- 国旗の baseURL は `PUBLIC_BASE_URL` 変数。未設定ならリクエストの `https://{Host}` から導く。

#### 返信文言と絵文字

返信文言（「入力」は正規化後の `text`。改行は `\n`）:

- 国名ヒット: 1 通目 text「[入力] ですね🤔」、2 通目 text「わかります{笑顔系}」、3 通目 国の詳細テンプレート
- 国名ミス: 1 通目 text「[入力] ですね🤔」、2 通目 text「む、むむっ、、わかりませんでした{悲しみ系}」
- 国の詳細テンプレート: buttons。title = 入力（クイズ正解時は正解の国の表示名）、text =「首都ja / 首都en」、altText = title、画像 = 国旗（クイズ正解時は画像なし）、アクションは uri「Wikipedia」と uri「Google Maps」の 2 つ
- クイズ: buttons。title「クイズです」、text「この国はどこでしょうか？」、altText「この国はどこでしょうか？」、画像 = 正解の国旗、postback 4 つ（ラベル = 各国の表示名）
- クイズ回答（postback）: 1 通目 text「[名] ですね?」、2 通目 text「む、むむっ、、\n正解です{祝福系}」または「む、むむっ、、\n残念！はずれです{残念系}」。正解時のみ 3 通目に国の詳細テンプレート、4 通目に confirm（text・altText「もう一度やりますか？」、message アクション「いいえ」→ 送信文字列 "いいえ"、「はい」→ "クイズ"）
- 非テキスト: text「テキストメッセージ以外わからないです」、sticker(packageId "446", stickerId "1999")
- 自己紹介: buttons。title = PROFILE_TITLE、text = PROFILE_TEXT、altText = PROFILE_TITLE、画像 = PROFILE_IMAGE_URL、uri アクション（ラベル PROFILE_LINK_TEXT → PROFILE_LINK_URL）
- 🤔 は思案系（U+1F914）の固定

絵文字グループ（各グループから一様ランダムに 1 つ。乱数は注入し、テストは固定シード）:

| グループ | コードポイント |
|---|---|
| 祝福系 | U+1F60A, U+1F60D, U+1F31F, U+2728, U+1F389, U+1F38A |
| 残念系 | U+1F61B, U+1F61C, U+1F61D, U+274C |
| 笑顔系 | U+1F600, U+1F60E, U+1F917, U+1F60F, U+1F609 |
| 悲しみ系 | U+1F622, U+1F62D, U+1F631, U+1F635, U+1F616 |
| 思案系 | U+1F914 |

元 bot は残念系で U+1F61D を重複して持っていたため、舌出しウインクを U+1F61C に直している。

### HTTP ハンドラ（internal/app）

- `POST {WEBHOOK_PATH}`（既定 `/webhook`）のみ Webhook を受ける。他メソッドは 405、他パスは 404。`GET /` は 200 で短い識別文字列を返す（疎通用）。
- 処理順: ボディ全読み → 署名検証（失敗は 400、ボディは「Invalid signature.」）→ JSON 解析（失敗は 400）→ 各イベントを順に処理 → 200。
- 返信はリクエスト内で同期的に送る。返信 API の失敗（400 の使い切り reply token、検証用のダミートークンなど）はログに残して 200 を返す。1 イベントの失敗は他イベントの処理を止めない。
- `isRedelivery` は特別扱いしない（1 分以内なら reply token は使える）。
- ログにユーザーの発話本文やトークンを出さない。イベント種別、国コード、返信 API のステータスのみ。
- 設定はリクエスト処理時に `Getenv` から読む（workers-go では環境束縛がリクエスト文脈で渡るため、`init` では読まない）。

### Workers エントリ（cmd/worker）

- `workers.Serve(app.NewHandler(deps))`。`deps.Getenv = cloudflare.Getenv`、`deps.Doer` は `fetch.NewClient()` を包むアダプタで、`*http.Request` のメソッド・URL・ヘッダー・ボディを `fetch.NewRequest` に移して `Do` する。
- 実装時に GOMODCACHE 内の workers-go v0.36.0 のソースで `fetch.Request`・`Client.Do`・`cloudflare.Getenv` の実際のシグネチャを確認し、差異があれば計画の記述より実装を優先する。
- ビルド: `go run github.com/syumai/workers-go/cmd/workers-assets-gen -mode=go && GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o ./build/app.wasm ./cmd/worker`。wrangler の `main` は `./build/worker.mjs`、`build.command` は `npm run build`。
- サイズ上限: 無料プランの圧縮後 3MB。`npx wrangler deploy --dry-run --outdir=dist` が表示する gzip サイズを 2.5MiB 以下に収める。超えた場合の手順は「リスク」参照。

### 設定項目（wrangler.jsonc の vars とシークレット）

| 名前 | 種別 | 必須 | 既定 |
|---|---|---|---|
| CHANNEL_SECRET | secret | 必須 | なし |
| CHANNEL_ACCESS_TOKEN | secret | 必須 | なし |
| WEBHOOK_PATH | var | 任意 | /webhook |
| PUBLIC_BASE_URL | var | 任意 | リクエストの Host から導出 |
| LINE_API_BASE_URL | var | 任意 | https://api.line.me |
| PROFILE_IMAGE_URL / PROFILE_TITLE / PROFILE_TEXT / PROFILE_LINK_TEXT / PROFILE_LINK_URL | var | 任意 | 空（自己紹介無効） |

ローカルは `.dev.vars`（gitignore）に書く。`.dev.vars.example` を同梱する。

### 代替案を選ばなかった理由

- LINE 公式 Go SDK: 生成コードが大きくスクリプトサイズ上限を圧迫する。必要なのは署名検証と 7 種類の JSON 型だけで、自前実装のほうが小さく検証しやすい。
- 返信を `cloudflare.WaitUntil` で非同期化: 応答は速くなるが、ホストテストで送信可否を観測しにくく、エラーも見えにくい。処理は数十ミリ秒で終わる想定なので同期送信にする。必要なら後から差し替えられるよう送信は 1 関数に閉じる。
- Wikidata ラベルをそのまま表示名にする: 正式名と通称が混在し、クイズの選択肢として不自然になる。ja.wikipedia 記事タイトルは日本語での標準的な呼び名に揃っていて URL とも一致する。

## タスク分解

各タスクは「テストを書く → 失敗を確認 → 実装 → 通過 → gofmt/vet → commit」の順で進める。1 タスク 1 コミットを目安にする。

| # | タスク | 依存 |
|---|---|---|
| 1 | 足場: go.mod（module `github.com/shimabox/line_country_bot_go`、`go 1.27.3`、workers-go v0.36.0）、package.json（wrangler を devDependencies、scripts: build/dev/deploy:dry）、wrangler.jsonc、.gitignore、.dev.vars.example、LICENSE（MIT）、README 骨子。`go version` が 1.27.3 に切り替わることと、空のテストで `go test ./...` が通ることを確認 | - |
| 2 | internal/line 署名検証: 正しい署名で true、改ざんボディ・ヘッダー欠落・Base64 不正・別シークレットで false、空ボディでも検証できる | 1 |
| 3 | internal/line Webhook 解析: testdata に `events: []`、text、image、postback、follow、未知 type、isRedelivery の JSON を置き、各フィールドの読み取りと未知型の非エラーを確認 | 1 |
| 4 | internal/line メッセージ型: text/sticker/buttons/confirm と 3 アクションの `json.Marshal` をゴールデン JSON と比較。omitempty の挙動（画像なし buttons）、`TruncateUTF16` のサロゲートペア境界 | 1 |
| 5 | internal/line 返信クライアント: 偽 Doer で URL・ヘッダー・ボディを検査、2xx で nil、4xx/5xx でステータス付きエラー、6 件以上でエラー、`LINE_API_BASE_URL` 差し替え | 4 |
| 6 | internal/country モデル・索引・URL: 小さなフィクスチャ JSON を embed したテスト用ローダで、正規化一致検索（表示名・別名・英語名小文字）、不一致で nil、`PickN` の重複なし・件数不足時全件、国旗/地図/Wikipedia URL の生成、首都なしの表現 | 1 |
| 7 | tools/gencountries 変換ロジック: SPARQL 結果 JSON のフィクスチャ（複数首都、首都なし、記事なし、ラベル 20 文字超、都・市の接尾）→ レコード集約・overrides 適用・検証エラー・決定的出力をテスト。その後 Wikidata に実行して `internal/country/data/countries.json` と初回 overrides.json（ZA, BO, IL, PS, NL など）を生成し、件数と検証結果を記録 | 6 |
| 8 | tools/fetchflags: URL/保存先の対応、既存スキップ、欠落列挙をテスト（HTTP は httptest）。実行して public/flags に全コード分を取得し、合計サイズと欠落なしを確認 | 7 |
| 9 | internal/bot 正規化とトリガー: 全角スペース・改行除去、ASCII 小文字化、「クイズ」「quiz」「QUIZ」「君の名は」「who」「いいえ」の判定、プロフィール無効時に「君の名は」が国名検索に落ちること | 6 |
| 10 | internal/bot 返信組み立て: 固定シード乱数で、非テキスト（sticker の packageId "446" / stickerId "1999" を期待値に含める）・自己紹介・クイズ（4 択、正解 1 件、data 形式、画像 URL）・国名ヒット/ミス（絵文字がグループ内）・postback 正解/不正解（件数 4/2、confirm の 2 アクション）・postback 不正データ・索引にない正解名・40/60 文字切り詰め | 4, 9 |
| 11 | internal/app ハンドラ: httptest で GET / 200、他パス 404、GET webhook 405、署名不正 400、JSON 不正 400、`events: []` 200、text イベントで偽 Doer が正しい返信を受ける、返信 API 失敗でも 200、複数イベントの独立処理、`WEBHOOK_PATH`/`PUBLIC_BASE_URL` の反映 | 5, 10 |
| 12 | cmd/worker: fetch アダプタと Getenv 配線、`GOOS=js GOARCH=wasm go vet ./...` と `npm run build` の成功、`wrangler deploy --dry-run` の gzip サイズ記録。超過時はリスク欄の手順 | 11 |
| 13 | ローカル疎通: `.dev.vars` にダミー値、`LINE_API_BASE_URL` をローカルのモック（node の http.createServer ワンライナー等）に向け、`npx wrangler dev` に対して署名付き curl で 7 シナリオ（疎通・text ヒット・text ミス・クイズ・postback 正解・非テキスト・署名不正）を実行し、ステータスとモックが受けた JSON を確認。`GET /flags/jp.png` が 200 image/png で返ることも確認 | 8, 12 |
| 14 | README とドキュメント: 概要、必要なもの（LINE 公式アカウント作成→Messaging API 有効化、Developer Trial は現存しない旨）、設定項目表、ビルド・開発・デプロイ手順、Webhook URL の設定と「応答メッセージ」をオフにする注意、データ更新手順（gencountries → fetchflags → テスト）、出典とライセンス、制限事項。NOTICE に出典（Wikidata: CC0、Flagpedia/flagcdn.com: パブリックドメイン）を記す | 13 |
| 15 | CI: .github/workflows/ci.yml（setup-go with go-version-file、`go vet ./...`、`go test -race ./...`、`GOOS=js GOARCH=wasm go vet ./...`、`npm ci && npm run build`、`wrangler deploy --dry-run` によるサイズ確認、gofmt と `go mod tidy` の差分なし）。push 後に動く前提で、ローカルでは同じコマンド列を通す | 12 |
| 16 | 総仕上げ: 完了条件を上から順に実行して結果を記録、不要ファイル確認。履歴の書き換えはしない（履歴の整理は検収後に采配役が行う） | 13, 14, 15 |

## 完了条件・受け入れ基準

- [ ] `go version` が go1.27.3 を報告し、go.mod の `go` ディレクティブが 1.27.3 である
- [ ] ホストで `gofmt -l .` が空、`go vet ./...` と `go test -race ./...` が成功、`go mod tidy` で差分が出ない
- [ ] `GOOS=js GOARCH=wasm go vet ./...` が成功し、`npm run build` で build/app.wasm と build/worker.mjs ができる
- [ ] `npx wrangler deploy --dry-run --outdir=dist` が成功し、表示される gzip サイズが 2.5MiB 以下（上限 3MB）
- [ ] countries.json は 190 件以上、コードは一意の大文字 2 文字、全件に `name_ja`（UTF-16 で 20 以下）と `wikipedia_ja_url` があり、首都が複数の国は overrides で 1 つに解決されている。生成は 2 回実行して差分が `generated_at` 以外に出ない
- [ ] public/flags に countries.json の全コード分の PNG があり、合計 1MB 未満。欠落をテストで検出できる
- [ ] 元 bot の 8 機能（国名→国情報、クイズ、クイズ回答、再挑戦確認、キャンセル語、自己紹介、非テキスト返信、トリガー別名 quiz/who）がすべて internal/bot のテストで固定されている
- [ ] 署名不正 400、JSON 不正 400、`events: []` 200、返信 API 失敗時 200、未知イベント無視が internal/app のテストで固定されている
- [ ] 1 回の返信メッセージ数が 5 を超えないことをクライアントのテストとクイズ回答のテストで保証している
- [ ] `wrangler dev` のローカル疎通 7 シナリオの結果（ステータスとモックが受けた JSON の要点）が最終報告に記載されている
- [ ] README に設定・ビルド・デプロイ・データ更新・出典が揃い、NOTICE に出典（Wikidata: CC0、Flagpedia/flagcdn.com: パブリックドメイン）があり、どちらにも個人環境のパスやシークレットが含まれない
- [ ] CI 定義が存在し、同じコマンド列をローカルで通している（GitHub 上の実行は push 後）
- [ ] コード・ログ・テストデータに実トークン、実シークレット、個人名が含まれない

## 未確定事項・リスクと判断の委ね方

計画承認時に確定した事項:

- 表示名: ja.wikipedia の記事タイトル。記事がない国は Wikidata の ja ラベル。overrides.json で上書きできる。
- 首都が複数の国: overrides.json で採用する首都の QID を 1 つ指定し、未指定なら生成を失敗させる。選定根拠を overrides のコメント欄（`note` フィールド）に残す。
- 設定名: 新規命名（CHANNEL_SECRET、CHANNEL_ACCESS_TOKEN、WEBHOOK_PATH、PUBLIC_BASE_URL、LINE_API_BASE_URL、PROFILE_IMAGE_URL ほか）。名前は 1 か所の定数に集約する。
- 国旗の表示: `imageSize: contain` と白背景。
- 英語名検索: 含める（en ラベル、大文字小文字を無視した完全一致）。
- CI: GitHub Actions の定義を追加する。
- スタンプ: 非テキストへの返信は packageId 446 / stickerId 1999（元 bot の 1/13 は現行の送信可能一覧にないため変更）

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| スクリプトサイズ | Go の wasm は net/http・encoding/json を含むと raw 5〜8MB、gzip 1.5〜2.5MB になりうる | 2.5MiB 超なら順に試す: (1) `-ldflags="-s -w" -trimpath` 確認、(2) binaryen の `wasm-opt -Oz` を build に追加、(3) 不要な標準パッケージ依存の削減。3MB を超えて解消できなければ止まって報告（TinyGo 切替は方針変更のため利用者判断） |
| CPU 時間 | 無料プランは 1 リクエスト 10ms CPU。Go ランタイム込みの処理時間は実測が必要 | `wrangler dev` では正確に測れないので、README の制限事項に記載し、デプロイ後の確認項目として引き継ぐ |
| workers-go API | fetch パッケージと Getenv の正確なシグネチャは実装時にモジュールソースで確認する | 差異があればソースを正とし、報告に記載。API が想定と大きく違い `http.Request` を素直に移せない場合は止まって報告 |
| Wikidata の揺れ | 再生成でラベルや首都が変わり、ボタンラベル超過や首都複数化で生成が失敗することがある | 生成失敗はエラーメッセージに対象コードと理由を出し、overrides で解決する手順を README に書く |
| ネットワーク | gencountries と fetchflags は Wikidata・flagcdn への読み取りアクセスが必要 | 実装環境で外部アクセスが拒否された場合は、変換ロジックのテストまで完了させ、生成物の作成を未完として報告 |
| 無料メッセージ数 | 返信は reply token を使うので課金対象のメッセージ数には数えない | push API は実装しない。設計上の不変条件として README に記載 |

## 公開経路

1. 実装担当は作業ブランチ plan/2026-10-24-go-workers-rewrite にタスク単位で commit する。GitHub への push はしない。
2. 実装レビュー（gpt-6.1-sol、read-only）の指摘を同ブランチで修正する。
3. 利用者の承認後、GitHub に private リポジトリ shimabox/line_country_bot_go を作成して push し、main への統合は PR で行う（CI がここで初めて動く）。
4. デプロイは利用者が行う: `npx wrangler login`、`npx wrangler secret put CHANNEL_SECRET`、同 `CHANNEL_ACCESS_TOKEN`、必要なら vars の設定、`npx wrangler deploy`。
5. LINE 側は利用者が行う: LINE 公式アカウント作成 → Messaging API 有効化 → チャネルシークレットとチャネルアクセストークン取得 → Webhook URL に `https://<worker>/webhook` を設定して「Webhook の利用」をオン → 検証ボタンで 200 を確認 → 公式アカウントマネージャーで「応答メッセージ」をオフ。
6. デプロイ後の確認（利用者）: 実機で 8 機能を確認、`wrangler tail` で返信 API のステータスを確認、CPU 時間の項目を見る。

3 以降は今回の実装範囲に含めない。3 は検収後に利用者の承認を得てから采配役が行う。
