# アーキテクチャ

LINE で国名を送ると国旗と首都を返し、国旗の 4 択クイズも出せる bot です。サーバーは持たず、Cloudflare Workers 上で Go を WebAssembly として動かしています。国データと国旗画像はビルド前に生成し、プログラムに同梱しています。

| 指標 | 値 | メモ |
|---|---|---|
| LINE への応答時間（本番） | 中央値 1.16 秒 | 上限 2 秒（最大 1.32 秒） |
| CPU 時間（本番） | 中央値 30ms（16〜97ms） | 待ち時間は含まない。立ち上がり直後の重いリクエストは中央値 56ms、それ以外は 18.5ms |
| wasm サイズ（gzip 後） | 652 KiB | TinyGo でビルド。無料プランの上限 3MB |
| 国データ / 国旗 | 196 / 196 | Wikidata（CC0）/ Flagpedia（パブリックドメイン） |

数値は 2026-10-25 時点の計測です。本番の値は TinyGo のビルドをデプロイしたあと、実機から送った 31 件の `wrangler tail` の記録から出しています。

## 全体の構成：メッセージが返信になるまで

![LINE アプリからのメッセージは LINE Platform から Webhook で Cloudflare Workers に届き、Workers がリクエストごとに起動する Go で書いたプログラム（TinyGo で wasm にビルド）が、ローディング表示と返信を LINE Messaging API に送る。国旗画像は Workers の静的ファイルから LINE アプリが直接取得する。](architecture/overview.svg)

LINE アプリから届いたメッセージは、LINE Platform から Webhook で Workers に渡ります。Workers はリクエストのたびに Go の wasm（TinyGo でビルド）を起動し、返答を決めてから、Messaging API に「…」の表示と返信を送ります。国旗画像は、返信に含めた URL を使って LINE アプリが Workers の静的ファイルから直接取得します（破線）。

## 1 回のリクエストの時間配分

![Webhook を受けてから 200 を返すまで約 1.16 秒。そのうち約 1 秒は待機で、CPU 時間に数えられるのは最初の処理の数十ミリ秒だけ。LINE のタイムアウト 2 秒まで余裕がある。](architecture/request-timing.svg)

待機は Webhook への応答を返す前に行います。使っているライブラリ workers-go v0.36 では、応答を返したあとに待つ処理が不安定なためです。待っている時間は CPU 時間に数えられません。待機の上限を 1.2 秒にして、通信と処理を足しても LINE の 2 秒の制限に収めています。グループや複数人のトークでは「…」を出せないので、待たずに返信します。

## Go のパッケージ構成

![cmd/worker が internal/app を起動し、app が line で署名検証と送信を、bot で返答の組み立てを行う。bot は country の国データと line のメッセージ型を使う。](architecture/packages.svg)

| パッケージ | 役割 |
|---|---|
| `cmd/worker` | Workers との接続（環境変数の読み取りと fetch）。Workers に依存するのはここだけ |
| `internal/app` | HTTP の受け口、設定、待機、ログ |
| `internal/bot` | 入力の判定と返答の組み立て（国名・クイズ・回答・自己紹介） |
| `internal/line` | 署名検証、Webhook の解析、メッセージ型、返信とローディング表示の API |
| `internal/country` | 国データ（生成済みの Go の表）、名前の検索、国旗の表示方法 |

`cmd/worker` 以外は、手元（ホスト）の `go test ./...` でテストできます。外部とのやり取りは「HTTP 送信」と「環境変数の読み取り」の 2 か所に絞り、テストでは差し替えています。

## リポジトリの構成

パッケージ以外のツールや配信ファイルも含めた、ディレクトリの構成です。

```
cmd/worker/          Workers のエントリポイント（js/wasm 専用）
internal/line/       署名検証・Webhook 解析・メッセージ型・返信 API クライアント
internal/country/    国データ（data/countries.json から生成した Go の表）・検索・URL 生成
internal/bot/        入力の正規化・トリガー語・絵文字・返信の組み立て
internal/app/        http.Handler（ルーティング・署名検証・イベント処理）と設定
tools/gencountries/  Wikidata から countries.json を生成
tools/gencountrygo/  countries.json を Go のソース（internal/country/countries_gen.go）に変換
tools/fetchflags/    flagcdn.com から国旗 PNG を取得し、寸法ファイルを生成
public/flags/        国旗画像（Workers Static Assets で配信）
scripts/             ローカル疎通と CPU 時間の計測のスクリプト
```

## ビルドとデプロイの流れ

![Wikidata から国データ、Flagpedia から国旗画像をツールで取得し、Go のコードと画像としてリポジトリに置く。npm run build で TinyGo により wasm を作り、wrangler deploy で Cloudflare Workers に載せる。PR では GitHub Actions が go・tinygo-test・build の 3 ジョブ（Go のテスト、TinyGo でのテスト、wasm ビルドとサイズ確認）を並列に実行する。](architecture/build-and-deploy.svg)

国データと国旗は、ツールを手元で実行して作り、生成物をリポジトリに置きます。Wikidata への問い合わせはビルドのたびには行いません。生成物がずれたらテストが失敗し、再生成のコマンドを示します。wasm は TinyGo でビルドします（`npm run build`）。通常の Go よりファイルが小さく、リクエストごとの読み込みと初期化が軽いためです。TinyGo と wasm-opt は `scripts/install-tinygo.sh` が公式配布物を hash 検証して入れ、手元と CI で同じ版を使います。通常の Go のビルド（`npm run build:go`）も比較と退避用に残しています。デプロイは手元から `wrangler deploy` で行います。CI は go・tinygo-test・build の 3 ジョブを並列に実行して確認するだけで、デプロイはしません。各コマンドの意味や失敗時の振る舞いは [development.md](development.md) にあります。

| 生成物 | 作り方 |
|---|---|
| `internal/country/data/countries.json` | `go run ./tools/gencountries`（Wikidata の SPARQL と `tools/gencountries/overrides.json`） |
| `internal/country/countries_gen.go` | `go generate ./internal/country`（countries.json から生成） |
| `public/flags/*.png`、`internal/country/flagsizes_gen.go` | `go run ./tools/fetchflags`（Flagpedia から取得し、寸法を書き出す） |
| `build/app.wasm`、`build/worker.mjs`、`build/wasm_exec.js` | `npm run build`（TinyGo） |

国データと国旗の更新の手順と、生成が失敗したときの補正は [data-update.md](data-update.md) にあります。

## 設定

設定は、secret の `CHANNEL_SECRET` と `CHANNEL_ACCESS_TOKEN`（必須）と、`wrangler.jsonc` の var（任意）で行い、リクエストごとに読み込みます。項目の一覧と既定値、LINE チャネル側の設定は [configuration.md](configuration.md) にまとめています。

## 制限事項と設計上の決まり

- **CPU 時間**: 無料プランは 1 リクエストあたり CPU 10ms です。Go の wasm はリクエストごとに起動するため、wasm を小さくして読み込みと初期化を軽くできる TinyGo でビルドし、国データは起動時に解析しなくて済む生成済みの Go の表で持っています。TinyGo は reflect や標準ライブラリの対応範囲が通常の Go と異なるため、依存や `encoding/json` の使い方を変えたら `tinygo test ./internal/...`（CI でも実行）で挙動を確かめます。TinyGo はパニック時に goroutine のスタックトレースを出さず、`-no-debug` でビルドしているため位置情報も少なく、本番のパニックは原因を追いにくくなります。`wrangler dev` では CPU 時間を正確に測れないため、ビルド同士の比較は `npm run bench:cpu`（[CPU 時間の計測](development.md#cpu-時間の計測)）で行い、本番の値はデプロイ後に Cloudflare のダッシュボードや `wrangler tail` で確かめてください。
- **返信は reply token だけを使う**: push API は実装しません。返信は課金対象のメッセージ数に数えられないため、無料のメッセージ数を消費しません。これは設計上の不変条件で、push 系の送信を追加しない前提です。
- **返信は同期送信**: Webhook の処理中に返信 API を呼び、結果をログに残してから 200 を返します。返信 API が失敗しても（使用済みの reply token など）200 を返し、LINE からの再送を起こしません。
- **1 回の返信は最大 5 件**: クライアントが 6 件以上を拒否します。bot の返信は最大 4 件です。
- **状態を持たない**: KV や D1 などのストレージは使いません。クイズの正解は回答ボタンの postback データに入っています。

## 今後の検討事項

| 項目 | 状況 |
|---|---|
| CPU 時間 | 本番の中央値 30ms。無料プランの目安 10ms は超えているが、打ち切りは出ていない。リクエストの約半分は立ち上がり直後で重い（46〜97ms）。TinyGo の最適化オプション（`-gc=leaking` など）が次の候補 |
| TinyGo | 採用済み。Go でビルドしていたとき（11 件）と比べて、重いリクエストの中央値は 83 → 56ms、それ以外は 21.5 → 18.5ms |
| JSON 生成の軽量化 | 返信と入力中表示の本文は手書きのエンコーダで生成済み。Webhook の受信（`json.Unmarshal`）は未対応で、置き換えるなら fuzz テストと一緒に行う |
| 開発用ツールの脆弱性 | `npm audit` が wrangler の依存（undici）で 3 件。bot 本体には含まれない |
