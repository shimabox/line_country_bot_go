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
internal/country/    国データ（data/countries.json から生成した Go の表）・検索・URL 生成
internal/bot/        入力の正規化・トリガー語・絵文字・返信の組み立て
internal/app/        http.Handler（ルーティング・署名検証・イベント処理）と設定
tools/gencountries/  Wikidata から countries.json を生成
tools/gencountrygo/  countries.json を Go のソース（internal/country/countries_gen.go）に変換
tools/fetchflags/    flagcdn.com から国旗 PNG を取得し、寸法ファイルを生成
public/flags/        国旗画像（Workers Static Assets で配信）
scripts/             ローカル疎通と CPU 時間の計測のスクリプト
```

`cmd/worker` 以外はホストの `go test ./...` でテストできます。

## 必要なもの

- Go 1.27.3（`go.mod` の `go` ディレクティブ。手元の Go が古くても `GOTOOLCHAIN=auto` で自動的に取得されます）
- Node.js と npm（wrangler を devDependencies として入れます）
- TinyGo 0.43.0 と Binaryen（wasm-opt）version_133（wasm のビルドに使います。下の `scripts/install-tinygo.sh` で入れます）
- Cloudflare アカウント（Workers 無料プラン）
- LINE 公式アカウントと Messaging API チャネル
  1. LINE 公式アカウントを作成する
  2. LINE Official Account Manager の設定から Messaging API を有効にする（チャネルが LINE Developers コンソールに作られます）
  3. LINE Developers コンソールでチャネルシークレットを確認し、チャネルアクセストークン（長期）を発行する

  以前あった「Developer Trial」プランは現存しないため、通常の LINE 公式アカウントから始めます。

TinyGo と wasm-opt は、次のスクリプトで GitHub Releases の公式配布物を取得し、`scripts/toolchain.env` に固定した SHA-256 と照合してから、リポジトリ直下の `.tools/`（git 管理外）に展開します。対応しているのは Linux（x86_64）と macOS（Apple silicon）です。

```sh
bash scripts/install-tinygo.sh
```

同じ版が展開済みなら取得しないので、何度実行してもかまいません（CI も同じスクリプトを使います）。hash が一致しなければ展開せずに失敗します。版を上げるときは `scripts/toolchain.env` の版と hash を書き換えます。`npm run build` は `.tools/` のツールを自動で使い、なければ PATH 上の同じ版の `tinygo` と `wasm-opt` を使います。

## 設定項目

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

自己紹介は `PROFILE_IMAGE_URL` を含む 5 項目がすべて揃ったときだけ有効です。画像だけ設定して他が欠けていると、自己紹介を無効にしてログに警告を出します。

1 対 1 のトークで返答があるときは、ローディング表示 API でトーク画面に入力中の表示を出し、`REPLY_DELAY_MS` だけ待ってから返答をまとめて返信します。返答の内容と通数は変わらず、プッシュ送信は使いません。LINE は Webhook に 2 秒以内に応答しないとタイムアウトとして扱い、待機は Webhook への応答の前に行うため、待機は 1 回の Webhook につき、最初に返信するイベントの前の 1 回だけにし、`REPLY_DELAY_MS` の上限を 1200 にしています。上限を超える値は上限に、整数でない値や負の値は既定値にして、ログに警告を出します。グループトークと複数人トークでは API が使えないため、ローディング表示も待機もせずにすぐ返信します。ローディング表示に失敗したときも、待たずにそのまま返信します。

secret は `npx wrangler secret put` で登録し、ファイルには書きません。var は `wrangler.jsonc` の `vars` で設定します。
設定はリクエストごとに読み込みます。

## ビルドと開発

```sh
npm ci                           # wrangler を入れる
bash scripts/install-tinygo.sh   # TinyGo と wasm-opt を .tools/ に入れる
go test -race ./...              # ホストでテスト
tinygo test ./internal/...       # TinyGo でホストのテスト（.tools/tinygo/bin と .tools/binaryen/bin を PATH に入れて実行）
npm run build                    # TinyGo で build/app.wasm、build/worker.mjs、build/wasm_exec.js を作る
npm run build:go                 # 通常の Go でビルドする（比較と退避用）
npm run deploy:dry               # wrangler deploy --dry-run --outdir=dist（サイズ確認）
bash scripts/check-size.sh 1024  # 上と同じ dry-run を行い、gzip が 1024 KiB を超えたら失敗する（CI と同じ）
```

`npm run build` は `scripts/build.sh` を実行します。workers-go の `workers-assets-gen -mode=tinygo` で JS の足場（TinyGo 用の `wasm_exec.js` を含む）を作り、`tinygo build -target wasm -no-debug ./cmd/worker` で wasm をビルドします。ビルドの前に `tinygo` の版が `scripts/toolchain.env` と一致することと `wasm-opt` があることを確かめ、欠けていれば `scripts/install-tinygo.sh` の実行を促して失敗します。初回のビルドは TinyGo が標準ライブラリをビルドするため約 14 秒かかり、その分 `wrangler deploy`（デプロイ時に `npm run build` を実行する）の所要時間も延びます。2 回目以降はキャッシュが効きます。

`npm run build:go`（`WASM_BUILD=go bash scripts/build.sh`）は、従来どおり `workers-assets-gen -mode=go` と `GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w"` でビルドします。TinyGo のツールは使いません。`WASM_BUILD` は `tinygo`（既定）か `go` で、それ以外はエラーです。`bash scripts/check-toolchain-errors.sh` で、導入スクリプトと `build.sh` の失敗時の振る舞い（hash 不一致、ツールの欠如、不正な `WASM_BUILD`）を確かめられます（TinyGo の配布物を取得するため CI では実行しません）。

無料プランのスクリプトサイズ上限は圧縮後 3MB です。`deploy:dry` が表示する gzip サイズで確認します。TinyGo のビルドは gzip で約 650 KiB、Go のビルドは約 2,000 KiB です。CI は 1024 KiB を上限にして、誤って Go のビルドになったときに失敗させます（`check-size.sh` の引数を省くと上限は 2.5MiB）。

### ローカル疎通

```sh
cp .dev.vars.example .dev.vars
# .dev.vars を編集: ダミーの CHANNEL_SECRET / CHANNEL_ACCESS_TOKEN と
# LINE_API_BASE_URL=http://127.0.0.1:8788（モックの待ち受け先）
npx wrangler dev --ip 127.0.0.1 --port 8787     # 別のターミナルで
node scripts/local-smoke.mjs http://127.0.0.1:8787
```

`local-smoke.mjs` は返信 API とローディング表示 API のモックを `LINE_API_BASE_URL` のポートで立て、署名付きの Webhook を送って、疎通・国名の一致と不一致・クイズ・正解の回答・非テキスト・署名不正の 7 場面と、国旗の表示方法（日本は `cover`、イギリスは `contain`）、`/flags/jp.png` の配信を確かめます。返信する場面では、モックが「ローディング表示 API → 返信 API」の順に受け取ったことと、Webhook の送信から 200 の受信までが `REPLY_DELAY_MS` 以上かつ 2000ms 未満であることも確かめ、経過時間（`elapsedMs`）を出力します。`.dev.vars` は git 管理外で、実際の認証情報は書きません。

### CPU 時間の計測

```sh
npm run build
npm run bench:cpu -- build/app.wasm            # 既定は 12 回
npm run bench:cpu -- build/app.wasm 60         # 回数を指定する
npm run bench:cpu -- build/app.wasm --verbose  # 各回の値も標準エラーに出す
npm run bench:cpu -- /path/to/saved/app.wasm 60 --wasm-exec /path/to/saved/wasm_exec.js  # 退避したビルドを測る
```

`scripts/bench-cpu.mjs` は、`build/worker.mjs` と同じ手順で wasm を Node.js 上で動かし、`GET /`、署名付き Webhook の「日本」「クイズ」の 3 場面について、CPU 時間（user + system）の中央値を表と JSON で出します。

| 列 | 内容 |
|---|---|
| `start` | Go の wasm インスタンスを作り、`main` が準備を終えるまで（workers-go はリクエストごとにこれを行う） |
| `handle` | リクエストを渡して応答を読み終えるまで |
| `total` | 同じ回の `start` と `handle` の合計 |

場面は交互に実行し、各回の前に GC を強制します。各場面の先頭 2 回は JIT の準備として中央値から除きます。返信 API はローカルのモックに送り、認証情報はダミーです。Node.js の値は本番（workerd）より小さく出るため、絶対値ではなく、同じマシンで 2 つのビルドを比べる用途に使います。wasm は、ビルドしたツールチェーンの `wasm_exec.js` と組にしないと動きません（TinyGo のビルドには TinyGo 用、Go のビルドには Go 用が要り、取り違えるとインポートの不一致で失敗します）。`--wasm-exec` を省くと `build/wasm_exec.js`、次に Go の `$(go env GOROOT)/lib/wasm/wasm_exec.js` を使います。比べるときは、比較したいビルド（例: `npm run build:go` の結果）の `build/app.wasm` と `build/wasm_exec.js` を一緒にリポジトリの外へ退避し、`--wasm-exec` で対応する `wasm_exec.js` を指定して、2 つのビルドを交互に数回ずつ計測して中央値を比べます。短い計測では JIT の最適化の進み具合で `handle` が偏ることがあるため、`handle` の比較には回数を増やした計測（60 回など）を使います。CI では実行しません。

## デプロイ

```sh
npx wrangler login
npx wrangler secret put CHANNEL_SECRET
npx wrangler secret put CHANNEL_ACCESS_TOKEN
# 必要なら wrangler.jsonc の vars（PUBLIC_BASE_URL や PROFILE_*）を編集
npx wrangler deploy
```

`wrangler deploy` は `wrangler.jsonc` の `build.command`（`npm run build`）で手元の TinyGo のビルドを作ってから載せるため、先に `bash scripts/install-tinygo.sh` を実行しておきます。TinyGo のビルドで問題が出たときは、`WASM_BUILD=go npx wrangler deploy` で通常の Go のビルドをデプロイすれば従来の状態に戻せます（wrangler が環境変数を `npm run build` に引き継ぎ、`scripts/build.sh` が Go のビルドを選びます）。

デプロイ後、LINE 側を設定します。

1. LINE Developers コンソールの Messaging API 設定で、Webhook URL に `https://<worker のホスト名>/webhook` を設定する
2. 「Webhook の利用」をオンにする
3. 「検証」ボタンで成功（200）になることを確かめる
4. LINE Official Account Manager の応答設定で「応答メッセージ」をオフにする（オンのままだと定型の自動応答が bot の返信と一緒に届きます）

動作は `npx wrangler tail` でログを見ながら確認できます。ログにはイベント種別・返信の種類・国コード・ローディング表示の結果・返信 API のステータスだけを出し、発話本文やトークン、利用者 ID は出しません。返信に失敗したときは、ステータス（応答がなければ 0）と固定の分類（`reply_api_error`: 返信 API が 2xx 以外を返した、`transport_error`: 送信できなかったか応答がなかった、`request_error`: 送信前に失敗した。アクセストークン未設定など）だけを出し、返信 API の応答ボディやエラーの文字列は出しません。ローディング表示の結果は `loading=ok`（表示した）、`loading=skipped`（1 対 1 のトークではない）、`loading=off`（`REPLY_DELAY_MS` が 0）、`loading=failed` のいずれかで、失敗したときは `loading_status` と `loading_error`（`loading_api_error`、`transport_error`、`request_error`）だけを出します。トーク画面を開いていない、友だちではない、ブロックされているなどの場合は、API が成功してもローディング表示は出ません。

## 国データと国旗の更新

国データは Wikidata、国旗は Flagpedia（flagcdn.com）から作ります。どちらもホストで実行します。

```sh
go run ./tools/gencountries        # Wikidata に問い合わせて internal/country/data/countries.json を作り直す
go generate ./internal/country     # countries.json から internal/country/countries_gen.go を作り直す
go run ./tools/fetchflags          # 足りない国旗を public/flags に取得（-force で全件取り直し）し、寸法ファイルを生成
go test ./...                      # データの上限検査、生成ファイル（国データの表・寸法ファイル）の鮮度検査、国旗の欠落検査
```

Worker は countries.json を読み込まず、`tools/gencountrygo` が countries.json から生成した Go の表（`internal/country/countries_gen.go`）を使います。JSON の解析と検索用の索引の構築を起動時に行わないためです。countries.json を正本とし、更新したら `go generate ./internal/country` で生成ファイルを作り直して、両方を commit します。生成ファイルが countries.json と食い違っていると、`go test ./internal/country` の `TestGeneratedMatchesJSON` が失敗します。`gencountrygo` は書き出す前に countries.json を検査し、不正なら何も書きません。

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

- **CPU 時間**: 無料プランは 1 リクエストあたり CPU 10ms です。Go の wasm はリクエストごとに起動するため、wasm を小さくして読み込みと初期化を軽くできる TinyGo でビルドし、国データは起動時に解析しなくて済む生成済みの Go の表で持っています。TinyGo は reflect や標準ライブラリの対応範囲が通常の Go と異なるため、依存や `encoding/json` の使い方を変えたら `tinygo test ./internal/...`（CI でも実行）で挙動を確かめます。TinyGo はパニック時に goroutine のスタックトレースを出さず、`-no-debug` でビルドしているため位置情報も少なく、本番のパニックは原因を追いにくくなります。`wrangler dev` では CPU 時間を正確に測れないため、ビルド同士の比較は `npm run bench:cpu`（「CPU 時間の計測」）で行い、本番の値はデプロイ後に Cloudflare のダッシュボードや `wrangler tail` で確かめてください。
- **返信は reply token だけを使う**: push API は実装しません。返信は課金対象のメッセージ数に数えられないため、無料のメッセージ数を消費しません。これは設計上の不変条件で、push 系の送信を追加しない前提です。
- **返信は同期送信**: Webhook の処理中に返信 API を呼び、結果をログに残してから 200 を返します。返信 API が失敗しても（使用済みの reply token など）200 を返し、LINE からの再送を起こしません。
- **1 回の返信は最大 5 件**: クライアントが 6 件以上を拒否します。bot の返信は最大 4 件です。
- **状態を持たない**: KV や D1 などのストレージは使いません。クイズの正解は回答ボタンの postback データに入っています。

## 出典とライセンス

- コード: [MIT License](LICENSE)
- 国データ: [Wikidata](https://www.wikidata.org/)（CC0 1.0）
- 国旗画像: [Flagpedia](https://flagpedia.net/)（[flagcdn.com](https://flagcdn.com/)、パブリックドメイン）

詳しくは [NOTICE](NOTICE) を参照してください。
