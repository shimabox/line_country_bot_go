# line_country_bot_go

国名を送ると国旗と首都を返し、国旗 4 択クイズも出せる LINE bot です。
Go で書かれ、Cloudflare Workers（Go → WebAssembly）の無料プランで動きます。
サーバーの運用や LINE SDK は不要で、国データと国旗はスクリプトを再実行するだけで更新できます。

## できること

| 送るもの | 返ってくるもの |
|---|---|
| 国名（例: 「日本」「アメリカ」「France」） | 「[日本] ですね🤔」「わかります😀」と、国旗・首都（日本語 / 英語）・Wikipedia と Google Maps へのボタン |
| 見つからない国名 | 「[〇〇] ですね🤔」「む、むむっ、、わかりませんでした😢」 |
| 「クイズ」「quiz」 | 国旗を見せて 4 択で国名を当てるクイズ |
| クイズの回答ボタン | 正誤。正解なら国の詳細と「もう一度やりますか？」（はい / いいえ） |
| 「いいえ」 | 返信なし（クイズの終了） |
| 「君の名は」「who」 | 自己紹介（プロフィールを設定したときだけ。未設定なら国名として検索） |
| スタンプや画像などテキスト以外 | 「テキストメッセージ以外わからないです」とスタンプ |

国名は前後や途中の空白（全角を含む）を無視し、英字は大文字小文字を区別せずに、表示名・日本語の別名・英語名と完全一致で検索します。

## 構成

```
cmd/worker/          Workers のエントリポイント（js/wasm 専用）
internal/line/       署名検証・Webhook 解析・メッセージ型・返信 API クライアント
internal/country/    国データ（data/countries.json を埋め込み）・検索・URL 生成
internal/bot/        入力の正規化・トリガー語・絵文字・返信の組み立て
internal/app/        http.Handler（ルーティング・署名検証・イベント処理）と設定
tools/gencountries/  Wikidata から countries.json を生成
tools/fetchflags/    flagcdn.com から国旗 PNG を取得し、寸法ファイルを生成
public/flags/        国旗画像（Workers Static Assets で配信）
scripts/             ローカル疎通スクリプト
```

`cmd/worker` 以外はホストの `go test ./...` でテストできます。

## 必要なもの

- Go 1.27.3（`go.mod` の `go` ディレクティブ。手元の Go が古くても `GOTOOLCHAIN=auto` で自動的に取得されます）
- Node.js と npm（wrangler を devDependencies として入れます）
- Cloudflare アカウント（Workers 無料プラン）
- LINE 公式アカウントと Messaging API チャネル
  1. LINE 公式アカウントを作成する
  2. LINE Official Account Manager の設定から Messaging API を有効にする（チャネルが LINE Developers コンソールに作られます）
  3. LINE Developers コンソールでチャネルシークレットを確認し、チャネルアクセストークン（長期）を発行する

  以前あった「Developer Trial」プランは現存しないため、通常の LINE 公式アカウントから始めます。

## 設定項目

| 名前 | 種別 | 必須 | 既定 | 説明 |
|---|---|---|---|---|
| `CHANNEL_SECRET` | secret | 必須 | なし | 署名検証に使うチャネルシークレット。未設定だと Webhook は 500 を返す |
| `CHANNEL_ACCESS_TOKEN` | secret | 必須 | なし | 返信 API に使うチャネルアクセストークン |
| `WEBHOOK_PATH` | var | 任意 | `/webhook` | Webhook を受けるパス |
| `PUBLIC_BASE_URL` | var | 任意 | リクエストの `https://{Host}` | 国旗画像の配信元（例: `https://line-country-bot-go.example.workers.dev`） |
| `LINE_API_BASE_URL` | var | 任意 | `https://api.line.me` | 返信 API の送信先。ローカル疎通でモックに向けるときに使う |
| `PROFILE_IMAGE_URL` | var | 任意 | 空 | 自己紹介の画像。空なら自己紹介は無効 |
| `PROFILE_TITLE` / `PROFILE_TEXT` | var | 任意 | 空 | 自己紹介のタイトルと本文 |
| `PROFILE_LINK_TEXT` / `PROFILE_LINK_URL` | var | 任意 | 空 | 自己紹介のボタンのラベルとリンク先 |

自己紹介は `PROFILE_IMAGE_URL` を含む 5 項目がすべて揃ったときだけ有効です。画像だけ設定して他が欠けていると、自己紹介を無効にしてログに警告を出します。

secret は `npx wrangler secret put` で登録し、ファイルには書きません。var は `wrangler.jsonc` の `vars` で設定します。
設定はリクエストごとに読み込みます。

## ビルドと開発

```sh
npm ci                 # wrangler を入れる
go test -race ./...    # ホストでテスト
npm run build          # build/app.wasm と build/worker.mjs を作る
npm run deploy:dry     # wrangler deploy --dry-run --outdir=dist（サイズ確認）
bash scripts/check-size.sh   # 上と同じ dry-run を行い、gzip が 2.5MiB を超えたら失敗する（CI と同じ）
```

`npm run build` は workers-go の `workers-assets-gen` で JS の足場を作り、`GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w"` で wasm をビルドします。
無料プランのスクリプトサイズ上限は圧縮後 3MB です。`deploy:dry` が表示する gzip サイズで確認します（2.5MiB 以下を目安にしています）。

### ローカル疎通

```sh
cp .dev.vars.example .dev.vars
# .dev.vars を編集: ダミーの CHANNEL_SECRET / CHANNEL_ACCESS_TOKEN と
# LINE_API_BASE_URL=http://127.0.0.1:8788（モックの待ち受け先）
npx wrangler dev --ip 127.0.0.1 --port 8787     # 別のターミナルで
node scripts/local-smoke.mjs http://127.0.0.1:8787
```

`local-smoke.mjs` は返信 API のモックを `LINE_API_BASE_URL` のポートで立て、署名付きの Webhook を送って、疎通・国名の一致と不一致・クイズ・正解の回答・非テキスト・署名不正の 7 場面と `/flags/jp.png` の配信を確かめます。`.dev.vars` は git 管理外で、実際の認証情報は書きません。

## デプロイ

```sh
npx wrangler login
npx wrangler secret put CHANNEL_SECRET
npx wrangler secret put CHANNEL_ACCESS_TOKEN
# 必要なら wrangler.jsonc の vars（PUBLIC_BASE_URL や PROFILE_*）を編集
npx wrangler deploy
```

デプロイ後、LINE 側を設定します。

1. LINE Developers コンソールの Messaging API 設定で、Webhook URL に `https://<worker のホスト名>/webhook` を設定する
2. 「Webhook の利用」をオンにする
3. 「検証」ボタンで成功（200）になることを確かめる
4. LINE Official Account Manager の応答設定で「応答メッセージ」をオフにする（オンのままだと定型の自動応答が bot の返信と一緒に届きます）

動作は `npx wrangler tail` でログを見ながら確認できます。ログにはイベント種別・返信の種類・国コード・返信 API のステータスだけを出し、発話本文やトークンは出しません。返信に失敗したときは、ステータス（応答がなければ 0）と固定の分類（`reply_api_error`: 返信 API が 2xx 以外を返した、`transport_error`: 送信できなかったか応答がなかった、`request_error`: 送信前に失敗した。アクセストークン未設定など）だけを出し、返信 API の応答ボディやエラーの文字列は出しません。

## 国データと国旗の更新

国データは Wikidata、国旗は Flagpedia（flagcdn.com）から作ります。どちらもホストで実行します。

```sh
go run ./tools/gencountries    # Wikidata に問い合わせて internal/country/data/countries.json を作り直す
go run ./tools/fetchflags      # 足りない国旗を public/flags に取得（-force で全件取り直し）し、寸法ファイルを生成
go test ./...                  # データの上限検査、国旗の欠落検査、寸法ファイルの鮮度検査
```

`fetchflags` は取得と検査のあと、`public/flags` の全国旗の幅と高さを PNG のヘッダから読み、`internal/country/flagsizes_gen.go` に書き出します（出力先は `-sizes` で変更でき、空文字なら生成しません）。既定の出力先と `-dir` はリポジトリ直下からの相対パスなので、リポジトリ直下で実行してください（出力先のディレクトリがなければ失敗します）。国旗が揃っていれば取得は行わないため、ネットワークなしで再生成できます。国旗を差し替えたり国データを更新したりしたら、再実行して生成ファイルも一緒に commit します（古いままだと `go test ./...` が失敗します）。返信では、この寸法から国旗を枠（rectangle、1.51:1）いっぱいに表示したときに幅または高さが切れる割合を求め、10% 以下なら `cover`（左右や上下の細い白い筋が出ない）、それより多く切れる旗（イギリス、スイス、ネパールなど）は `contain`（白背景で全体を表示）にします。閾値は `internal/country/flag.go` の定数です。

- 対象は Wikidata の主権国家（P31=Q3624078）のうち、ISO 3166-1 alpha-2 コード（P297）を持ち、解体（P576）されていないものです。クエリは `tools/gencountries/query.sparql` にあります。
- 表示名は ja.wikipedia の記事名（記事がなければ日本語ラベル）です。クイズのボタンに使うため 20 文字（UTF-16）以内に収める必要があります。
- 首都は P36（終了日のないもの）を使い、末尾の「都」「市」「特別市」を除きます（東京都 → 東京）。
- 出力はコード順で、再実行しても `generated_at` 以外は変わりません。

### 生成が失敗したとき

Wikidata の更新で首都が複数になったり、表示名が長くなったりすると、`gencountries` は対象の国コードと理由を一覧にして失敗します。`tools/gencountries/overrides.json` に補正を書いて再実行します。

```json
{
  "exclude": ["XX"],
  "countries": {
    "ZA": {"capital": "Q3926", "note": "採用した理由"},
    "GE": {"name_ja": "ジョージア", "note": "記事名の曖昧さ回避を外す"}
  }
}
```

| 項目 | 内容 |
|---|---|
| `exclude` | 出力から外す国コード |
| `qid` | 同じコードの項目が複数あるとき、採用する国の QID |
| `name_ja` | 表示名 |
| `aliases_ja` | 検索語の追加 |
| `remove_aliases` | 生成された別名のうち、誤解を招くものや他国と重なるものを外す |
| `wikipedia_ja_url` | Wikipedia の URL |
| `capital` | 首都が複数あるとき、採用する首都の QID（候補はエラーに表示されます）。首都が複数なら `capital_ja` や `lat` / `lon` を補正していても省略できません |
| `capital_ja` / `capital_en` | 首都の表示名 |
| `lat` / `lon` | 地図の座標 |
| `note` | 補正の理由（生成には使わない） |

存在しない国コードへの補正や知らない項目名もエラーになるので、古くなった補正や書き間違いに気づけます。複数の国が同じ別名を持つと、その別名では検索できません。`gencountries` は該当する別名を警告に出します。

国データを更新したら `fetchflags` で国旗を揃えます。国旗が足りないときや、どの国にも属さないファイルが `public/flags` にあるときは、`go test ./...` が失敗します。

## 制限事項と設計上の決まり

- **CPU 時間**: 無料プランは 1 リクエストあたり CPU 10ms です。Go の wasm はリクエストごとに起動し、埋め込みの国データを読み込みます。`wrangler dev` では CPU 時間を正確に測れないため、デプロイ後に Cloudflare のダッシュボードや `wrangler tail` で確かめてください。
- **返信は reply token だけを使う**: push API は実装しません。返信は課金対象のメッセージ数に数えられないため、無料のメッセージ数を消費しません。これは設計上の不変条件で、push 系の送信を追加しない前提です。
- **返信は同期送信**: Webhook の処理中に返信 API を呼び、結果をログに残してから 200 を返します。返信 API が失敗しても（使用済みの reply token など）200 を返し、LINE からの再送を起こしません。
- **1 回の返信は最大 5 件**: クライアントが 6 件以上を拒否します。bot の返信は最大 4 件です。
- **状態を持たない**: KV や D1 などのストレージは使いません。クイズの正解は回答ボタンの postback データに入っています。

## 出典とライセンス

- コード: [MIT License](LICENSE)
- 国データ: [Wikidata](https://www.wikidata.org/)（CC0 1.0）
- 国旗画像: [Flagpedia](https://flagpedia.net/)（[flagcdn.com](https://flagcdn.com/)、パブリックドメイン）

詳しくは [NOTICE](NOTICE) を参照してください。
