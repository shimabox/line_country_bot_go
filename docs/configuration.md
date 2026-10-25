# 設定

bot の概要と動かし方は [README](../README.md)、全体の構成は [architecture.md](architecture.md) を参照してください。

## 設定項目の一覧

| 名前 | 種別 | 必須 | 既定 | 説明 |
|---|---|---|---|---|
| `CHANNEL_SECRET` | secret | 必須 | なし | 署名検証に使うチャネルシークレット。未設定だと Webhook は 500 を返す |
| `CHANNEL_ACCESS_TOKEN` | secret | 必須 | なし | 返信 API に使うチャネルアクセストークン |
| `WEBHOOK_PATH` | var | 任意 | `/webhook` | Webhook を受けるパス |
| `PUBLIC_BASE_URL` | var | 任意 | リクエストの `https://{Host}` | 国旗画像の配信元（例: `https://line-country-bot-go.example.workers.dev`） |
| `LINE_API_BASE_URL` | var | 任意 | `https://api.line.me` | 返信 API とローディング表示 API の送信先。ローカル疎通でモックに向けるときに使う |
| `REPLY_DELAY_MS` | var | 任意 | `1000` | 返信の前にローディング表示（入力中の「…」）を出して待つ時間（ミリ秒）。上限 `1200`、`0` で表示も待機もせずすぐ返信する |
| `PROFILE_IMAGE_URL` | var | 任意 | 空 | 自己紹介の画像。空なら自己紹介は無効 |
| `PROFILE_TITLE` / `PROFILE_TEXT` | var | 任意 | 空 | 自己紹介のタイトルと本文 |
| `PROFILE_LINK_TEXT` / `PROFILE_LINK_URL` | var | 任意 | 空 | 自己紹介のボタンのラベルとリンク先 |

`CHANNEL_ACCESS_TOKEN` は返信とローディング表示の API に使い、長期のチャネルアクセストークンを設定します。発行の手順は [development.md](development.md) の「環境構築」にあります。

## 自己紹介の有効条件

自己紹介は `PROFILE_IMAGE_URL` を含む 5 項目がすべて揃ったときだけ有効です。画像だけ設定して他が欠けていると、自己紹介を無効にしてログに警告を出します。

## ローディング表示と待機

1 対 1 のトークで返答があるときは、ローディング表示 API でトーク画面に入力中の表示を出し、`REPLY_DELAY_MS` だけ待ってから返答をまとめて返信します。返答の内容と通数は変わらず、プッシュ送信は使いません。LINE は Webhook に 2 秒以内に応答しないとタイムアウトとして扱い、待機は Webhook への応答の前に行うため、待機は 1 回の Webhook につき、最初に返信するイベントの前の 1 回だけにし、`REPLY_DELAY_MS` の上限を 1200 にしています。上限を超える値は上限に、整数でない値や負の値は既定値にして、ログに警告を出します。グループトークと複数人トークでは API が使えないため、ローディング表示も待機もせずにすぐ返信します。ローディング表示に失敗したときも、待たずにそのまま返信します。

`REPLY_DELAY_MS` が 0 のとき、ログのローディング表示の結果は `loading=off` になります。ログに出す内容は [development.md](development.md) の「ログ」を参照してください。

## 設定の登録方法と読み込み

secret は `npx wrangler secret put` で登録し、ファイルには書きません。var は `wrangler.jsonc` の `vars` で設定します。
設定はリクエストごとに読み込みます。

## LINE チャネル側の設定

LINE チャネルの「セキュリティ設定」の IP アドレス制限は空にしています。Workers は送信元の IP アドレスが固定されないためです。

Webhook URL の設定と応答メッセージの切り替えは、[README](../README.md) の「デプロイ」にある手順で行います。
