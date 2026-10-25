# ビルドと開発

bot の概要と最短の動かし方は [README](../README.md)、全体の構成は [architecture.md](architecture.md) を参照してください。

## 環境構築

必要なものの一覧と版は [README](../README.md) の「必要なもの」にあります。

### LINE チャネルの作り方

LINE 公式アカウントと Messaging API チャネルの作り方は、[README](../README.md) の「デプロイ」にあります。

以前あった「Developer Trial」プランは現存しないため、通常の LINE 公式アカウントから始めます。

### TinyGo と wasm-opt の導入

TinyGo と wasm-opt は、次のスクリプトで GitHub Releases の公式配布物を取得し、`scripts/toolchain.env` に固定した SHA-256 と照合してから、リポジトリ直下の `.tools/`（git 管理外）に展開します。対応しているのは Linux（x86_64）と macOS（Apple silicon）です。

```sh
bash scripts/install-tinygo.sh
```

同じ版が展開済みなら取得しないので、何度実行してもかまいません（CI も同じスクリプトを使います）。hash が一致しなければ展開せずに失敗します。版を上げるときは `scripts/toolchain.env` の版と hash を書き換えます。`npm run build` は `.tools/` のツールを自動で使い、なければ PATH 上の同じ版の `tinygo` と `wasm-opt` を使います。

## コマンド一覧

```sh
npm ci                           # wrangler を入れる
bash scripts/install-tinygo.sh   # TinyGo と wasm-opt を .tools/ に入れる
gofmt -l $(git ls-files '*.go')  # 整形の確認（git 管理下の .go だけが対象で .tools/ は含まない。CI と同じ）
go test -race ./...              # ホストでテスト
tinygo test ./internal/...       # TinyGo でホストのテスト（.tools/tinygo/bin と .tools/binaryen/bin を PATH に入れて実行）
npm run build                    # TinyGo で build/app.wasm、build/worker.mjs、build/wasm_exec.js を作る
npm run build:go                 # 通常の Go でビルドする（比較と退避用）
npm run deploy:dry               # wrangler deploy --dry-run --outdir=dist（サイズ確認）
bash scripts/check-size.sh 1024  # 上と同じ dry-run を行い、gzip が 1024 KiB を超えたら失敗する（CI と同じ）
```

## ビルドの仕組み

`npm run build` は `scripts/build.sh` を実行します。workers-go の `workers-assets-gen -mode=tinygo` で JS の足場（TinyGo 用の `wasm_exec.js` を含む）を作り、`tinygo build -target wasm -no-debug -gc=leaking ./cmd/worker` で wasm をビルドします。ビルドの前に `tinygo` の版が `scripts/toolchain.env` と一致することと `wasm-opt` があることを確かめ、欠けていれば `scripts/install-tinygo.sh` の実行を促して失敗します。初回のビルドは TinyGo が標準ライブラリをビルドするため約 14 秒かかり、その分 `wrangler deploy`（デプロイ時に `npm run build` を実行する）の所要時間も延びます。2 回目以降はキャッシュが効きます。

`-gc=leaking` は、メモリを割り当てるだけで回収しない TinyGo の GC です。workers-go はリクエストごとに Go の wasm インスタンスを作って捨てるため、回収しなくてもメモリはリクエストをまたいで積み上がりません。GC の仕事がなくなる分だけ CPU 時間が減り、wasm も小さくなります（前提と実測値は [architecture.md の制限事項](architecture.md#制限事項と設計上の決まり)、計測値は [CPU 時間の計測](#cpu-時間の計測) にあります）。GC は環境変数 `TINYGO_GC` で選べ、値はそのまま TinyGo の `-gc` に渡します（既定は `leaking`）。`TINYGO_GC=precise npm run build` で、導入前と同じ TinyGo の既定の GC（wasm では `precise`）のビルドを作れます。不正な値は TinyGo のエラーで失敗します。`tinygo test` には `-gc=leaking` を付けません（ホストのテストは割り当てが多く、回収しないとメモリが増え続けるため）。

`npm run build:go`（`WASM_BUILD=go bash scripts/build.sh`）は、従来どおり `workers-assets-gen -mode=go` と `GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w"` でビルドします。TinyGo のツールは使いません。`WASM_BUILD` は `tinygo`（既定）か `go` で、それ以外はエラーです。`bash scripts/check-toolchain-errors.sh` で、導入スクリプトと `build.sh` の失敗時の振る舞い（hash 不一致、ツールの欠如、不正な `WASM_BUILD`）を確かめられます（TinyGo の配布物を取得するため CI では実行しません）。

## スクリプトサイズの上限

無料プランのスクリプトサイズ上限は圧縮後 3MB です。`deploy:dry` が表示する gzip サイズで確認します。TinyGo のビルドは gzip で約 553 KiB（`-gc=leaking`。導入前の GC では約 652 KiB）、Go のビルドは約 2,000 KiB です。CI は 1024 KiB を上限にして、誤って Go のビルドになったときに失敗させます（`check-size.sh` の引数を省くと上限は 2.5MiB）。

## ローカル疎通

```sh
cp .dev.vars.example .dev.vars
# .dev.vars を編集: ダミーの CHANNEL_SECRET / CHANNEL_ACCESS_TOKEN と
# LINE_API_BASE_URL=http://127.0.0.1:8788（モックの待ち受け先）
npx wrangler dev --ip 127.0.0.1 --port 8787     # 別のターミナルで
node scripts/local-smoke.mjs http://127.0.0.1:8787
```

`local-smoke.mjs` は返信 API とローディング表示 API のモックを `LINE_API_BASE_URL` のポートで立て、署名付きの Webhook を送って、疎通・国名の一致と不一致・クイズ・正解の回答・非テキスト・署名不正の 7 場面と、国旗の表示方法（日本は `cover`、イギリスは `contain`）、`/flags/jp.png` の配信を確かめます。返信する場面では、モックが「ローディング表示 API → 返信 API」の順に受け取ったことと、Webhook の送信から 200 の受信までが `REPLY_DELAY_MS` 以上かつ 2000ms 未満であることも確かめ、経過時間（`elapsedMs`）を出力します。`.dev.vars` は git 管理外で、実際の認証情報は書きません。

設定項目の意味と既定値は [configuration.md](configuration.md) にあります。

## CPU 時間の計測

```sh
npm run build
npm run bench:cpu -- build/app.wasm            # 既定は 12 回
npm run bench:cpu -- build/app.wasm 60         # 回数を指定する
npm run bench:cpu -- build/app.wasm --verbose  # 各回の値も標準エラーに出す
npm run bench:cpu -- /path/to/saved/app.wasm 60 --wasm-exec /path/to/saved/wasm_exec.js  # 退避したビルドを測る
npm run bench:cpu -- build/app.wasm 12 --events 20  # 1 回の Webhook に同じイベントを 20 件入れる
```

`scripts/bench-cpu.mjs` は、`build/worker.mjs` と同じ手順で wasm を Node.js 上で動かし、`GET /`、署名付き Webhook の「日本」「クイズ」の 3 場面について、CPU 時間（user + system）の中央値と wasm のメモリの最大値を表と JSON で出します。

| 列 | 内容 |
|---|---|
| `start` | Go の wasm インスタンスを作り、`main` が準備を終えるまで（workers-go はリクエストごとにこれを行う） |
| `handle` | リクエストを渡して応答を読み終えるまで |
| `total` | 同じ回の `start` と `handle` の合計 |
| `mem` | 各回の `handle` の後に読んだ wasm の線形メモリの大きさ（`memory.buffer.byteLength`）の、全回での最大値（MiB） |

wasm のメモリは段階的に増えて縮まないため、`mem` は 1 リクエストが実際に使った量の上限値です。測れるのは wasm の線形メモリだけで、JS 側のヒープは含みません。`--events N`（既定 1）は、「日本」と「クイズ」の Webhook に同じイベントを N 件（reply token は件ごとに変える）入れて 1 回のリクエストにします。件数が増えたときのメモリと CPU 時間を見るためのもので、`GET /` は変わりません。省くと従来と同じ場面です。

場面は交互に実行し、各回の前に GC を強制します。各場面の先頭 2 回は JIT の準備として中央値から除きます。返信 API はローカルのモックに送り、認証情報はダミーです。Node.js の値は本番（workerd）より小さく出るため、絶対値ではなく、同じマシンで 2 つのビルドを比べる用途に使います。wasm は、ビルドしたツールチェーンの `wasm_exec.js` と組にしないと動きません（TinyGo のビルドには TinyGo 用、Go のビルドには Go 用が要り、取り違えるとインポートの不一致で失敗します）。`--wasm-exec` を省くと `build/wasm_exec.js`、次に Go の `$(go env GOROOT)/lib/wasm/wasm_exec.js` を使います。比べるときは、比較したいビルド（例: `npm run build:go` の結果）の `build/app.wasm` と `build/wasm_exec.js` を一緒にリポジトリの外へ退避し、`--wasm-exec` で対応する `wasm_exec.js` を指定して、2 つのビルドを交互に数回ずつ計測して中央値を比べます。短い計測では JIT の最適化の進み具合で `handle` が偏ることがあるため、`handle` の比較には回数を増やした計測（60 回など）を使います。CI では実行しません。

### `-gc=leaking` の計測値

`-gc=leaking` の導入前（TinyGo の既定の GC、`precise`）と導入後のビルドを、同じマシンで 60 回ずつ交互に 3 巡計測した値です（2026-10-25、Node.js）。値は巡ごとの中央値の中央値で、単位は CPU ms です。

| 場面 | 導入前 `start` / `handle` / `total` | 導入後 `start` / `handle` / `total` | `total` の減少 |
|---|---|---|---|
| `GET /` | 4.15 / 3.79 / 8.25 | 3.89 / 1.25 / 5.44 | 34% |
| Webhook「日本」 | 3.57 / 22.51 / 25.96 | 3.38 / 13.12 / 16.19 | 38% |
| Webhook「クイズ」 | 3.96 / 23.26 / 26.92 | 3.59 / 13.28 / 16.71 | 38% |

減ったのは主に `handle`（GC の仕事）で、`start` はほぼ変わりません。3 巡のどの巡でも、導入後の Webhook の `total` は導入前の最小値を下回りました。

`mem`（MiB）は次のとおりです。

| 場面 | 導入前 | 導入後 |
|---|---|---|
| `GET /` | 1.25 | 1.25 |
| Webhook「日本」1 件 | 1.25 | 5.00 |
| Webhook「クイズ」1 件 | 1.25 | 5.00 |
| Webhook「日本」20 件（`--events 20`） | 2.50 | 40.00 |
| Webhook「クイズ」20 件（`--events 20`） | 2.50 | 40.00 |

導入後は割り当てた分だけメモリが増え、イベント 1 件あたり約 1.8 MiB（(40 − 5) ÷ 19）増えます。20 件のリクエストでは CPU 時間は減らず、やや増えました（`--events 20` で 12 回計測した `total` の中央値で、「日本」139.5 → 164.2ms、「クイズ」137.8 → 156.2ms）。CPU 時間が減るのはイベントが少ない通常のリクエストです。メモリの上限との関係は [architecture.md の制限事項](architecture.md#制限事項と設計上の決まり) にあります。

## デプロイの仕組みと切り戻し

`wrangler deploy` は `wrangler.jsonc` の `build.command`（`npm run build`）で手元の TinyGo のビルドを作ってから載せます。そのため、先に `bash scripts/install-tinygo.sh` を実行しておく必要があります。

TinyGo のビルドで問題が出たときは、`WASM_BUILD=go npx wrangler deploy` で通常の Go のビルドをデプロイすれば従来の状態に戻せます。wrangler が環境変数を `npm run build` に引き継ぎ、`scripts/build.sh` が Go のビルドを選びます。デプロイの手順は [README](../README.md) の「デプロイ」にあります。

`-gc=leaking` で問題が出たとき（`npx wrangler tail` の outcome に `exceededMemory` が出るなど）は、次のいずれかで戻します。

- `npx wrangler rollback`: 直前のデプロイに戻します。ビルドし直さないため最も早く戻せます
- `TINYGO_GC=precise npx wrangler deploy`: 今の commit のまま、導入前と同じ TinyGo の既定の GC でビルドしてデプロイします（`WASM_BUILD` と同じく、wrangler が環境変数を `npm run build` に引き継ぎます）
- `-gc=leaking` の導入前の commit を checkout して `npx wrangler deploy`

切り戻したまま運用を続けるときは、docs の説明とデプロイしたビルドがずれないよう、`scripts/build.sh` の `TINYGO_GC` の既定を戻します。

## ログ

動作は `npx wrangler tail` でログを見ながら確認できます。ログにはイベント種別・返信の種類・国コード・ローディング表示の結果・返信 API のステータスだけを出し、発話本文やトークン、利用者 ID は出しません。返信に失敗したときは、ステータス（応答がなければ 0）と固定の分類（`reply_api_error`: 返信 API が 2xx 以外を返した、`transport_error`: 送信できなかったか応答がなかった、`request_error`: 送信前に失敗した。アクセストークン未設定など）だけを出し、返信 API の応答ボディやエラーの文字列は出しません。ローディング表示の結果は `loading=ok`（表示した）、`loading=skipped`（1 対 1 のトークではない）、`loading=off`（`REPLY_DELAY_MS` が 0）、`loading=failed` のいずれかで、失敗したときは `loading_status` と `loading_error`（`loading_api_error`、`transport_error`、`request_error`）だけを出します。トーク画面を開いていない、友だちではない、ブロックされているなどの場合は、API が成功してもローディング表示は出ません。
