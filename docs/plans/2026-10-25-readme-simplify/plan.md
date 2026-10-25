# README の簡素化と docs への移設 実装計画

- 日付: 2026-10-25
- リポジトリ: github.com/shimabox/line_country_bot_go
- ベースブランチ: main
- ベース SHA: c04f61c644f51fab8b5290b7077d93b102e15976
- 作業ブランチ: plan/2026-10-25-readme-simplify
- 起案担当: 予定 claude-fable-5-1 / high。確認済み: claude-fable-5-1（確認源: CLI 結果 JSON の modelUsage）、effort は未確認
- 計画レビュー担当: 予定 gpt-6-astra / high（Codex CLI, read-only。未実行）
- 編成の例外理由: なし
- 計画書の作成担当: claude-opus-5-5 / medium（起案と同じプロバイダー）
- 実装担当: claude-opus-5-5 / high（文書の移設と相互リンクの整合が中心。コードの変更はない）
- 実装レビュー担当: 予定 gpt-6.1-sol / high（Codex CLI, read-only）。通常段で 3 回合格しなければ実装 claude-fable-5-1 / high、レビュー gpt-6-astra / high に昇格
- 計画書の扱い: レビュー・privacy 検査後に docs/plans/2026-10-25-readme-simplify/plan.md と request.md を commit
- 公開範囲: 実装担当は作業ブランチへの commit まで。push・PR 作成と CI の確認は検収後に采配役（利用者が承認済み）。マージは利用者が自分で行う。本番デプロイは関係しない

## 背景・目的

README.md は約 220 行あり、「何をする bot か」と「動かし方」のあいだに、設定の全項目、ビルドの内部、CPU 時間の計測、国データの生成とその補正、ログの形式、設計上の決まりが同じ階層で並んでいます。初めて見る人は入口を見つけにくく、開発者は目的の節を探しにくい状態です。また docs/architecture.md と README に設定とビルドの流れの重複があり、片方だけ更新されてずれる余地があります。

この変更で、README は「何をする bot か・友だち追加の方法・必要なもの・最短の動かし方・デプロイ・詳しい文書への案内・出典」だけになり、詳細は読者の目的別に docs/ 配下へ移ります。情報は移すだけで失いません。設定とビルドの流れは docs 側の 1 か所を正本にして重複を解消します。

## スコープ

- 対象:
  - README.md の再構成（残す節の選別と並び替え、docs への案内の追加、友だち追加の節の新設）
  - 友だち追加の QR 画像 docs/images/line-qr.png の追加（LINE 公式の QR 画像。後述）
  - 実際の LINE の画面のキャプチャ docs/images/quiz-screenshot.png の追加（利用者が用意した画像。後述）
  - 新規文書 docs/configuration.md、docs/development.md、docs/data-update.md の作成
  - docs/architecture.md への追記（リポジトリの構成、制限事項と設計上の決まり）と「設定」節の正本一本化
  - README・docs 間の相互リンクの整備
  - docs/plans/2026-10-25-readme-simplify/ に plan.md と request.md を追加
- 対象外:
  - コード・スクリプト・CI・wrangler.jsonc・.dev.vars.example・package.json・NOTICE・LICENSE の変更
  - docs/plans 配下の既存ファイルの変更
  - docs/architecture.md の計測値・図・「今後の検討事項」の内容の変更
  - 英語版の作成
  - 事実の追加や更新（計測し直し、版の確認など）。文書に書かれていることの範囲で移す

## 方針

### README に残す内容と並び

次の順で、目安 70〜100 行（上限 120 行）にします。

1. タイトルと冒頭 3 文（現 1〜5 行）。そのまま
1a. 「友だち追加」（新設）。bot を試したい人がすぐ見つけられるよう、冒頭のすぐ下に置く。説明文は「次のリンクを開くか、QR コードを LINE で読み取って友だちに追加すると試せます。友だちになってください。」とする。友だち追加のリンク https://line.me/R/ti/p/@aed1744s と、QR 画像 `docs/images/line-qr.png` を幅 180px 程度で表示する（`<img src="docs/images/line-qr.png" width="180" alt="…">` のように HTML で幅を指定してよい。alt には「LINE で友だち追加する QR コード」などの説明を書く）。QR 画像はリンク先と同じ友だち追加先を指す
2. 「できること」（現 7〜19 行）。節の先頭（表の前）に画面のキャプチャ `docs/images/quiz-screenshot.png` を幅 320px 程度で表示する（`<img src="docs/images/quiz-screenshot.png" width="320" alt="…">` の形でよい。alt には「クイズを送ると国旗と 4 択が返ってくる LINE の画面」のように内容が分かる説明を書く）。表と検索の説明（現 9〜19 行）は連続した原文のまま残す
3. 「必要なもの」。Go 1.27.3、Node.js と npm、TinyGo 0.43.0 と Binaryen version_133、Cloudflare アカウント、LINE 公式アカウントと Messaging API チャネルの 5 項目を箇条書きで残す。版番号と `GOTOOLCHAIN=auto` の補足は残す。「LINE 公式アカウントと Messaging API チャネル」の項目の案内先は README の「デプロイ」（同じ README 内）にする。LINE アカウントの作成手順は README「デプロイ」の先頭へ、Developer Trial の注記とツールチェーン導入の仕組みは docs/development.md へ
4. 「動かすまで」。次の 4 コマンドだけを残し、他のコマンドは docs/development.md へ案内する

   ```sh
   npm ci                           # wrangler を入れる
   bash scripts/install-tinygo.sh   # TinyGo と wasm-opt を .tools/ に入れる
   go test -race ./...              # ホストでテスト
   npm run build                    # TinyGo で wasm をビルドする
   ```

5. 「デプロイ」。README だけ読んでデプロイまで通せるよう、次の順に置く。README には「やること」だけを残し、仕組みの説明は docs/development.md へ
   - 先頭（コマンドブロックの前）: 先に LINE 側を用意する旨と、現 45〜49 行の 3 手順（LINE 公式アカウントを作成する / LINE Official Account Manager の設定から Messaging API を有効にする（チャネルが LINE Developers コンソールに作られます）/ LINE Developers コンソールでチャネルシークレットを確認し、チャネルアクセストークン（長期）を発行する）。続けて「`npx wrangler secret put` を実行すると値を聞かれるので、控えたチャネルシークレットとチャネルアクセストークンをそれぞれ入力します」の 1 文。`npx wrangler secret put CHANNEL_SECRET` / `CHANNEL_ACCESS_TOKEN` で入れる値は、LINE 側でチャネルを作ってから LINE Developers コンソールで確認・発行するものだから
   - コマンドブロック（現 134〜140 行）。原文のまま
   - コマンドブロックの後: 「`npx wrangler deploy` はその場で TinyGo のビルド（`npm run build`）を作ってから載せるので、先に `bash scripts/install-tinygo.sh` を済ませておきます。」と、「TinyGo のビルドで問題が出たときは、次のコマンドで通常の Go のビルドに戻せます。」とコードブロック `WASM_BUILD=go npx wrangler deploy`（文言はこのとおり、または同じ意味で短く）。現 142 行目の仕組みの説明は README に置かない
   - LINE 側の設定 4 手順（現 144〜149 行）。原文のまま
   - ログの説明（現 151 行）は「動作は `npx wrangler tail` で確認できます」の 1 文と docs/development.md へのリンクにする
6. 「ドキュメント」。docs の 4 文書を 1 行ずつの説明付きで列挙する
7. 「出典とライセンス」（現 212〜218 行）。そのまま

### 友だち追加の QR 画像

- 画像は LINE 公式の QR 画像（`https://qr-official.line.me/sid/L/aed1744s.png`、PNG 360×360、1,680 byte、SHA-256 393ba61050efd7861778be0ca7944ad99062a68b5c9cf5d534101dabeb60e5e7）をそのまま `docs/images/line-qr.png` に置く。加工・圧縮はしない。友だち追加のリンク https://line.me/R/ti/p/@aed1744s と同じ友だち追加先を指す（利用者が確認済み）
- 画像ファイルは采配役が取得して実行時添付情報で渡す。実装担当はネットワークから取り直さず、渡されたファイルをコピーしてハッシュを照合する

### 画面のキャプチャ

- 画像は実際の LINE の画面のキャプチャ（クイズを送ったときの返答。PNG 1080×1248、80,931 byte、SHA-256 ab2148c02fce26971f4105dd17b0fddbd4d7d582ac7f9cb779be9aeb7cf71a8a）で、利用者が用意したもの。そのまま `docs/images/quiz-screenshot.png` に置く。加工・圧縮はしない
- 写っているのは bot の表示名・クイズのカード・既読と時刻だけで、個人を特定できる情報はない（采配役が確認済み）
- 画像ファイルは采配役が実行時添付情報で渡す。実装担当は渡されたファイルをコピーしてハッシュを照合する

### docs 側の文書構成

| ファイル | 見出し構成（`##`） | 由来 |
|---|---|---|
| docs/configuration.md（新規）`# 設定` | 設定項目の一覧（表） / 自己紹介の有効条件 / ローディング表示と待機 / 設定の登録方法と読み込み / LINE チャネル側の設定 | README 59〜78 行。architecture.md「設定」の IP アドレス制限の段落と「長期トークン」の補足を取り込む |
| docs/development.md（新規）`# ビルドと開発` | 環境構築（LINE チャネルの作り方は手順の本文を置かず README「デプロイ」へのリンクで案内し、Developer Trial の注記を残す。TinyGo と wasm-opt の導入） / コマンド一覧 / ビルドの仕組み（`build.sh`、`WASM_BUILD`、`check-toolchain-errors.sh`） / スクリプトサイズの上限 / ローカル疎通 / CPU 時間の計測 / デプロイの仕組みと切り戻し（現 142 行目の仕組みの説明を本文として置く） / ログ | README 49（Developer Trial の注記）、51〜57、80〜130、142（仕組みの説明）、151 行 |
| docs/data-update.md（新規）`# 国データと国旗の更新` | 更新の手順 / 生成ファイルと正本 / 国旗の取得と表示方法 / 対象と表示名の決まり / 生成が失敗したとき（overrides.json） | README 153〜202 行。見出しの階層は現状の `##` と `###` を `##` に揃える |
| docs/architecture.md（既存） | 「Go のパッケージ構成」の直後に「リポジトリの構成」（README 23〜36 行のツリーと `go test` の補足）を追加。「設定」節は 2 文と docs/configuration.md へのリンクに置き換え。「今後の検討事項」の前に「制限事項と設計上の決まり」（README 204〜210 行）を追加。「ビルドとデプロイの流れ」の段落末と生成物の表の後に docs/development.md・docs/data-update.md へのリンクを足す | README 21〜36、204〜210 行 |

ファイル名は既存の architecture.md に合わせて英語の小文字、本文は日本語です。

### README の各節の行き先（対応表）

| 現 README の行 | 節 | 行き先 | 扱い |
|---|---|---|---|
| 1〜5 | 冒頭 | README | そのまま |
| 7〜19 | できること | README | そのまま。節の先頭（表の前）に画面のキャプチャの行を加え、表と検索の説明（9〜19 行）は連続した原文のまま残す |
| 21〜36 | 構成 | architecture.md「リポジトリの構成」 | 移す。36 行目は architecture.md の同趣旨の文と統合してよい |
| 38〜44 | 必要なもの（箇条書き） | README | 残す。42 行目の「下のスクリプトで入れます」は docs/development.md への案内に書き換える。「LINE 公式アカウントと Messaging API チャネル」の項目の案内先は README「デプロイ」にする |
| 45〜49 | LINE 公式アカウントの手順 | README「デプロイ」の先頭（3 手順）。Developer Trial の注記は development.md「環境構築」 | 移す。development.md「環境構築」の LINE チャネルの作り方は README へのリンクにし、手順の本文は重複させない |
| 51〜57 | ツールチェーン導入の仕組み | development.md「環境構築」 | 移す。54 行目のコマンドは README「動かすまで」にも残す |
| 59〜78 | 設定項目 | configuration.md | 移す。README には「`CHANNEL_SECRET` と `CHANNEL_ACCESS_TOKEN` が必須。他は任意」の 1〜2 文とリンク |
| 80〜92 | コマンドブロック | development.md「コマンド一覧」 | 全行を移す。83・84・86・88 行目は README にも残す |
| 94〜98 | ビルドの仕組み・サイズ上限 | development.md | 移す |
| 100〜110 | ローカル疎通 | development.md | 移す |
| 112〜130 | CPU 時間の計測 | development.md | 移す |
| 132〜140 | デプロイのコマンド | README | 残す。コマンドブロック（134〜140 行）は連続した行として原文のまま。見出しとの間に LINE 側の用意を置く |
| 142 | deploy の仕組みと切り戻し | README「デプロイ」（やることだけ）と development.md「デプロイの仕組みと切り戻し」（仕組み）に分ける | README には先に導入スクリプトを済ませることと `WASM_BUILD=go npx wrangler deploy` のコードブロックを置く。development.md には `wrangler.jsonc` の `build.command`（`npm run build`）が手元でビルドすること、wrangler が環境変数を `npm run build` に引き継ぎ `scripts/build.sh` が Go のビルドを選ぶことを本文として置く。README の切り戻しコマンドへのリンクは残してよい |
| 144〜149 | LINE 側の設定 | README | 連続した行として原文のまま |
| 151 | ログの説明 | development.md「ログ」 | 移す。README には `npx wrangler tail` の 1 文とリンク |
| 153〜202 | 国データと国旗の更新 | data-update.md | 移す |
| 204〜210 | 制限事項と設計上の決まり | architecture.md「制限事項と設計上の決まり」 | 移す。206 行目の「CPU 時間の計測」の参照は docs/development.md の該当節へのリンクにする |
| 212〜218 | 出典とライセンス | README | そのまま |

### 重複の解消

- 設定: configuration.md を正本にする。architecture.md の「設定」の表は削除し、表にあって README になかった補足（`CHANNEL_ACCESS_TOKEN` が長期トークンであること、LINE チャネルの IP アドレス制限を空にしている理由）は configuration.md に取り込む。
- ビルドの流れ: architecture.md の「ビルドとデプロイの流れ」は図と概要の段落と生成物の表を残す。コマンドの意味や失敗時の振る舞いは development.md と data-update.md に置き、architecture.md からリンクする。
- 構成: README のツリー（tools/、public/、scripts/ を含む）は architecture.md のパッケージ表にない情報を含むため、表の後ろにツリーをそのまま置く。
- 出典: README の「出典とライセンス」と data-update.md 冒頭の「Wikidata・Flagpedia から作る」は役割が違うため両方残す。

### 相互リンク

- README から docs へは `docs/<file>.md` の相対リンク。docs から README へは `../README.md`。
- 節へのリンクはファイル単位を基本にし、アンカーは「CPU 時間の計測」など元の README が名前で参照していた箇所だけに使う。アンカーは GitHub の規則（小文字化、空白をハイフンに、記号を除去。日本語はそのまま残る）で作る。例: `docs/development.md#cpu-時間の計測`。
- docs の各新規ファイル冒頭に、README と architecture.md への 1 行の案内を置く。
- 図の相対パス（`architecture/*.svg`）は変更しない。

### 文体

です・ます、短い文、表とコードブロックを保つ。移す文は原文を基本にし、節の分割に伴う接続詞や指示語（「下の」「上と同じ」）だけ直す。コマンドのコードブロックはコメントも含めて原文のまま複写する。

### 代替案と選ばなかった理由

- すべて architecture.md に寄せる: 1 ファイルが 400 行を超え、README の問題を docs に移すだけになる。
- 節ごとに 6〜7 ファイルに分ける（operations.md、benchmark.md、toolchain.md など）: ファイルが短くなりすぎ、README の「ドキュメント」の案内が長くなる。ログと CPU 計測は開発時に見るものなので development.md にまとめる。
- README に設定表を残す: 必須は 2 項目だけで、残りは任意。全項目の表は動かすまでの読者には不要。
- README を箇条書きの要約だけにしてコマンドを全部 docs に移す: 「最短の動かし方」が README から消え、利用者の要望に反する。

## タスク分解

| # | タスク | 依存 |
|---|---|---|
| 1 | ベース SHA から作業ブランチ plan/2026-10-25-readme-simplify を作る。計画書 2 点を docs/plans/2026-10-25-readme-simplify/ に置く | - |
| 2 | 検証用に移動前の README の語彙一覧を作る（受け入れ基準のスクリプト。出力はリポジトリ外に置く） | 1 |
| 3 | docs/configuration.md を作る（README 59〜78 行、architecture.md「設定」の補足を統合） | 1 |
| 4 | docs/data-update.md を作る（README 153〜202 行） | 1 |
| 5 | docs/development.md を作る（README 49 行の Developer Trial の注記、51〜57、80〜130、142 行の仕組みの説明、151 行。LINE チャネルの作り方は README「デプロイ」へのリンクで案内） | 1 |
| 6 | docs/architecture.md を編集する（リポジトリの構成の追加、設定節の置き換え、制限事項の追加、リンクの追加） | 3, 4, 5 |
| 7 | README.md を再構成する（残す節、「動かすまで」「ドキュメント」の新設、「できること」の先頭への画面のキャプチャの追加、「デプロイ」の先頭への LINE 側の用意と、コマンドブロック後のやることの追加、docs へのリンク） | 3, 4, 5, 6 |
| 8 | 相互リンクとアンカーを通し、元の README 内の節参照（206 行目など）を直す | 7 |
| 9 | 受け入れ基準の機械的な確認を実行し、MISSING を 0 にする。行数・フェンスの対応・docs/plans の差分を確認する | 8 |
| 10 | 移動前の README と新しい README・docs を節ごとに突き合わせ、対応表の全行に行き先があることを確認する | 9 |
| 11 | commit する（docs の新規作成・architecture.md の変更・README の再構成を 1〜3 commit に分ける。計画書は別 commit） | 10 |

## 完了条件・受け入れ基準

- [ ] 変更がドキュメントだけであること。`git diff --name-only <ベース SHA>` の出力が README.md、docs/architecture.md、docs/configuration.md、docs/development.md、docs/data-update.md、docs/images/line-qr.png、docs/images/quiz-screenshot.png、docs/plans/2026-10-25-readme-simplify/ の 2 ファイルの合計 9 ファイルだけ
- [ ] README の冒頭のすぐ下に「友だち追加」の節があり、説明文が「次のリンクを開くか、QR コードを LINE で読み取って友だちに追加すると試せます。友だちになってください。」で、リンク https://line.me/R/ti/p/@aed1744s と QR 画像（`docs/images/line-qr.png`、alt 付き）がある。`docs/images/line-qr.png` の SHA-256 が 393ba61050efd7861778be0ca7944ad99062a68b5c9cf5d534101dabeb60e5e7 と一致する
- [ ] README「できること」の表の前に画面のキャプチャ（`docs/images/quiz-screenshot.png`、alt 付き）がある。`docs/images/quiz-screenshot.png` の SHA-256 が ab2148c02fce26971f4105dd17b0fddbd4d7d582ac7f9cb779be9aeb7cf71a8a と一致する
- [ ] README「デプロイ」の先頭（コマンドブロックの前）に、LINE 側の用意の 3 手順と「`npx wrangler secret put` を実行すると値を聞かれるので、控えたチャネルシークレットとチャネルアクセストークンをそれぞれ入力します」の 1 文がある。`WASM_BUILD=go npx wrangler deploy` がコードブロックとして README にある
- [ ] docs/development.md「環境構築」に Developer Trial の注記があり、LINE チャネルの作り方は README へのリンクで、3 手順の本文を持たない。「デプロイの仕組みと切り戻し」に現 142 行目の仕組みの説明が本文としてある
- [ ] docs/plans の既存ファイルに変更・削除・改名がないこと

  ```sh
  git diff --name-only --diff-filter=MDR <ベース SHA> -- docs/plans   # 出力が空
  ```

- [ ] README.md が 120 行以下（目安 70〜100 行）で、見出しが「友だち追加」「できること」「必要なもの」「動かすまで」「デプロイ」「ドキュメント」「出典とライセンス」の順
- [ ] 情報が失われていないこと。移動前の README に含まれるコードスパン、大文字の識別子（環境変数名・ログの分類）、URL、コードブロックの各行（末尾のコメントを除く）が、移動後の README か docs（docs/plans を除く）のどこかに完全一致で存在する。次の確認で MISSING が 0 件。0 件にできないものは、同じ事実が別表現で残っている箇所を完了報告に 1 件ずつ示し、レビュー担当が承認する

  ```sh
  BASE=c04f61c644f51fab8b5290b7077d93b102e15976
  git show "$BASE:README.md" > /tmp/readme_old.md
  {
    grep -oE '`[^`]+`' /tmp/readme_old.md | tr -d '`'
    grep -oE '\b[A-Z][A-Z0-9_]{2,}\b' /tmp/readme_old.md
    grep -oE 'https?://[^ )>`]+' /tmp/readme_old.md
  } | sort -u > /tmp/tokens.txt
  awk '/^```/{f=!f; next} f' /tmp/readme_old.md | sed -E 's/[[:space:]]+#.*$//' | grep -v '^[[:space:]]*$' | sort -u > /tmp/cmds.txt
  cat /tmp/tokens.txt /tmp/cmds.txt | while IFS= read -r t; do
    git grep --untracked -qF -- "$t" -- README.md docs ':(exclude)docs/plans' || echo "MISSING: $t"
  done
  ```

- [ ] 単位付きの数値が失われていないこと。次の一覧の各項目が移動後の README か docs に存在する（手動で突き合わせ、完了報告に結果を書く）

  ```sh
  grep -oE '[0-9][0-9,.]*[[:space:]]*(ms|秒|KiB|MB|MiB|文字|回|件|%|項目|場面)' /tmp/readme_old.md | sort -u
  ```

- [ ] 対応表の全行について、行き先のファイルと節を完了報告に記す。「そのまま」の節は移動前後の該当行の diff が空であること（冒頭、出典とライセンス）。できることは、表と検索の説明（現 9〜19 行）が連続した行として原文のまま残ること（先頭に画面のキャプチャの行が加わるため）。デプロイのコマンドブロック（現 134〜140 行）と LINE 側の設定 4 手順（現 144〜149 行）は、それぞれ連続した行として原文のまま残ること（前後に文が加わってもよい）
- [ ] コードブロック内のコマンドは原文と同一であること（上の cmds.txt の確認に加え、コメントも含めた行単位で移動後のファイルに存在する）
- [ ] 相対リンクが切れていないこと。README.md と docs/*.md のローカルリンクと img の src（`http` で始まらないもの）について、`#` より前のパスがリンク元ファイルからの相対で存在する

  ```sh
  for f in README.md docs/*.md; do
    d=$(dirname "$f")
    {
      grep -oE '\]\([^)]+\)' "$f" | sed -E 's/^\]\((.*)\)$/\1/'
      grep -oE 'src="[^"]+"' "$f" | sed -E 's/^src="(.*)"$/\1/'
    } | grep -vE '^https?://' | cut -d'#' -f1 | grep -v '^$' |
    while IFS= read -r p; do [ -e "$d/$p" ] || echo "BROKEN: $f -> $p"; done
  done
  ```

- [ ] アンカー付きリンクの各アンカーが、リンク先の見出しから GitHub の規則（小文字化、空白をハイフンに、英数字・ハイフン・空白・日本語以外の記号を除去）で作ったものと一致する。使い捨てのスクリプトか手作業で全件を確認し、一覧を完了報告に書く。PR 作成後、采配役が GitHub の表示でもクリックして確認する
- [ ] README・docs の各ファイルでコードフェンス（```）の数が偶数であること
- [ ] 文体の確認: 新規文書と追記部分がです・ます調で、見出し階層が `#` 1 つから始まり飛びがないこと
- [ ] docs/architecture.md の計測値・図・「今後の検討事項」に差分がないこと（`git diff` で該当節に変更がない）
- [ ] `gofmt -l $(git ls-files '*.go')` と `go test ./...` がベースと同じ結果であること（コードに触れていない確認）
- [ ] PR の CI（go・tinygo-test・build の各 workflow のうち、この変更で起動するもの）が成功すること。采配役が PR 作成後に確認する

## 未確定事項・リスクと判断の委ね方

| 項目 | 内容 | 実装時の扱い |
|---|---|---|
| docs の分割粒度 | 確定: 新規 3 ファイル（configuration、development、data-update）+ architecture.md 追記（利用者が選択） | この構成で進める。development.md が 150 行を超えても分けない（超えたら完了報告に行数を書く） |
| README に残すコマンド | 案 A: 動かすまで 4 つ + デプロイ 5 つ（推奨。ほかに切り戻しの `WASM_BUILD=go npx wrangler deploy` のコードブロックを置く）。案 B: デプロイだけ。案 C: 現 82〜92 行の全部 | A で進める。`npx wrangler dev` を加えるかは実装担当の判断でよい（加えるなら `.dev.vars` の準備が要ることを 1 文添える） |
| 142 行目（切り戻し）の置き場所 | 確定（PR を見た利用者が承認）: README「デプロイ」には「やること」（先に導入スクリプトを済ませること、`WASM_BUILD=go npx wrangler deploy` での切り戻し）だけを置き、仕組みの説明は development.md「デプロイの仕組みと切り戻し」に本文として置く | この分け方で進める。development.md から README の切り戻しコマンドへのリンクは残してよい |
| LINE チャネルの用意の置き場所 | 確定（PR を見た利用者が承認）: README「デプロイ」の先頭に 3 手順と secret put の 1 文を置く。Developer Trial の注記は development.md「環境構築」に残す | development.md の LINE チャネルの作り方は README へのリンクにし、手順の本文を重複させない |
| ログの説明の置き場所 | development.md「ログ」（推奨）か、configuration.md の `REPLY_DELAY_MS` の近く | development.md で進める。`loading=off` と `REPLY_DELAY_MS` の関係は configuration.md からリンクで触れる |
| 出典とライセンス | README に残す（推奨。GitHub の慣例で、NOTICE と LICENSE へのリンクが入口にある）。docs/license.md に分ける案は選ばない | README に残す |
| 英語版 | 作らない（推奨）。作るなら別 PR | 作らない |
| architecture.md「設定」節の扱い | 案 A: 表を削除しリンクのみ（推奨。二重管理をなくす）。案 B: 要約表を残す | A で進める。削除する表の補足は configuration.md に取り込む |
| 日本語見出しのアンカー | GitHub の slug 規則が日本語で期待どおりか、PR 作成前は表示で確かめられない | アンカー付きリンクは最小限にする。確認は受け入れ基準どおり、PR 作成後に采配役が表示で行う |
| リスク: 情報の取りこぼし | 長い段落（75、130、151、166 行目）は文を分けて移すときに 1 文が落ちやすい | 原文を段落ごと複写してから整える。受け入れ基準の語彙確認と節ごとの突き合わせで検出する |
| リスク: コマンドの誤転記 | 引数や版番号（`1024`、`0.43.0`、`version_133`、`--wasm-exec`）の誤り | コードブロックはエディタの複写で移し、手打ちしない。cmds.txt の確認で検出する |
| リスク: リンク切れ | ファイル名の変更やアンカーの誤り | 受け入れ基準のスクリプトで検出する。ファイル名は本計画のものから変えない |
| リスク: README が短すぎる | 「動かすまで」が docs を読まないと完結しない | 必要なものと動かすまでの手順だけで `npm run build` まで到達できることを、README だけを読んで確認する |
| CI の起動条件 | workflow に paths フィルタがあると docs 変更では何も走らない可能性がある | 実装担当は `.github/workflows` を読んで起動条件を完了報告に書く。走らない場合は手元の `go test ./...` の結果を代わりに示す |

## 公開経路

1. 実装担当が作業ブランチ plan/2026-10-25-readme-simplify に commit する（push しない）
2. 実装レビューと検収のあと、采配役が push し、main 向けの PR を 1 本作る（文書のみ）
3. 采配役が CI の結果とリンク・アンカーの表示を確認して止める
4. マージは利用者が行う。本番デプロイは関係しない（README と docs の変更は Workers に載らない）
