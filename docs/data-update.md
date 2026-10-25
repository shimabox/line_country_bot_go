# 国データと国旗の更新

bot の概要と動かし方は [README](../README.md)、生成物とビルドの流れは [architecture.md](architecture.md) を参照してください。

国データは Wikidata、国旗は Flagpedia（flagcdn.com）から作ります。どちらもホストで実行します。

## 更新の手順

```sh
go run ./tools/gencountries        # Wikidata に問い合わせて internal/country/data/countries.json を作り直す
go generate ./internal/country     # countries.json から internal/country/countries_gen.go を作り直す
go run ./tools/fetchflags          # 足りない国旗を public/flags に取得（-force で全件取り直し）し、寸法ファイルを生成
go test ./...                      # データの上限検査、生成ファイル（国データの表・寸法ファイル）の鮮度検査、国旗の欠落検査
```

国データを更新したら `fetchflags` で国旗を揃えます。国旗が足りないときや、どの国にも属さないファイルが `public/flags` にあるときは、`go test ./...` が失敗します。

## 生成ファイルと正本

Worker は countries.json を読み込まず、`tools/gencountrygo` が countries.json から生成した Go の表（`internal/country/countries_gen.go`）を使います。JSON の解析と検索用の索引の構築を起動時に行わないためです。countries.json を正本とし、更新したら `go generate ./internal/country` で生成ファイルを作り直して、両方を commit します。生成ファイルが countries.json と食い違っていると、`go test ./internal/country` の `TestGeneratedMatchesJSON` が失敗します。`gencountrygo` は書き出す前に countries.json を検査し、不正なら何も書きません。

## 国旗の取得と表示方法

`fetchflags` は取得と検査のあと、`public/flags` の全国旗の幅と高さを PNG のヘッダから読み、`internal/country/flagsizes_gen.go` に書き出します（出力先は `-sizes` で変更でき、空文字なら生成しません）。既定の出力先と `-dir` はリポジトリ直下からの相対パスなので、リポジトリ直下で実行してください（出力先のディレクトリがなければ失敗します）。国旗が揃っていれば取得は行わないため、ネットワークなしで再生成できます。国旗を差し替えたり国データを更新したりしたら、再実行して生成ファイルも一緒に commit します（古いままだと `go test ./...` が失敗します）。返信では、この寸法から国旗を枠（rectangle、1.51:1）いっぱいに表示したときに幅または高さが切れる割合を求め、10% 以下なら `cover`（左右や上下の細い白い筋が出ない）、それより多く切れる旗（イギリス、スイス、ネパールなど）は `contain`（白背景で全体を表示）にします。閾値は `internal/country/flag.go` の定数です。

## 対象と表示名の決まり

- 対象は Wikidata の主権国家（P31=Q3624078）のうち、ISO 3166-1 alpha-2 コード（P297）を持ち、解体（P576）されていないものです。クエリは `tools/gencountries/query.sparql` にあります。
- 表示名は ja.wikipedia の記事名（記事がなければ日本語ラベル）です。クイズのボタンに使うため 20 文字（UTF-16）以内に収める必要があります。
- 首都は P36（終了日のないもの）を使い、末尾の「都」「市」「特別市」を除きます（東京都 → 東京）。
- 出力はコード順で、再実行しても `generated_at` 以外は変わりません。

## 生成が失敗したとき（overrides.json）

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
