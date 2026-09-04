# 0006: `config` / `model` を公開し、設定の YAML 往復と kind ごとの append-only を契約する

## ステータス

Accepted

採択日: 2026-09-04

## 日付

2026-09-04

## コンテキスト

DocDag の設定は当初 CLI の内部型だった。`internal/config` の `Config` とその下位型、`ADRPreset` /
`SpecPreset`、`Load` / `Merge` / `Validate` が文書グラフの契約そのものであり、`docdag.yaml` はそれを
YAML で書いたものに過ぎない。上位のリポジトリが vault 設定を Go で組み立て、生成した
`docdag.yaml` を CI で再生成との差分ゼロで固定したいとき、その型を import できなければ同じ契約を
二度書くことになる。

一方で CLI の入出力（`--version` の一行、`query --binding` の JSON、`validate` / `lint` の exit 符号）は
既に外部のハーネスが依存しており、公開は破壊的変更であってはならない。また設定の妥当性検査は
既に `Config.Validate` にあり、`Resolve` がファイル探索とマージの後にそれを呼ぶ。検証入口を新設する
必要はなく、どれが安定でどれが CLI 都合かを書く必要がある。

`--immutable-since` は単一 kind・単一ディレクトリを前提にしていた。多 kind コーパス全体を拒否する
のではなく、機械が書く kind だけ append-only にしたい。

## 決定

1. **`github.com/Kaikei-e/DocDag/config` と `.../model` をリポジトリ直下の公開パッケージにする。**
   `pkg/` は掘らない。`internal/` に再輸出は残さない。
2. **安定 API** は次に限る。破るときはこの記録を supersede する。
   - `Config` と下位型の**フィールド名と YAML タグ**
   - `ADRPreset` / `SpecPreset` / `Preset`
   - `Load` / `Merge` / `Validate`
   - `model.Severity` と `SeverityError` / `SeverityWarn` / `SeverityInfo`
   - `lint.Check` が返す `model.Finding` の**読むフィールド**（Severity、Rule、ID、Detail、Location）
3. **公開はするが互換を約束しないもの**: `Options`、`Resolve`、`Discover`、`DiscoveryPaths`、
   `IDNormalizer` と実装群、`IDPattern`、`StrictModality`、`Prohibition`、`model.Graph`、
   `internal/lint` の広い入口（`Run` / `Options` / `Locator`）。
4. **YAML 往復を契約する。** `yaml.Marshal(cfg)` の結果を `Load` して `Validate` し、
   `reflect.DeepEqual` で元の値と一致すること。同じ入力の Marshal はバイト単位で決定的であること。
   `EdgeCondition` は閾値が既定（少なくとも 1、上限なし）のときスカラ形を書き、そうでなければ
   マッピング形を書く。
5. **薄い公開 lint 入口** `lint.Check(cfg, vaultDir, fixturesDir)` をリポジトリ直下の `lint` に置く。
   `vaultDir` 空なら DNF 層のみ。`internal/lint` は動かさない。
6. **`KindSpec.append_only`**（YAML `append_only: true`）。多 kind の `--immutable-since` は
   それを付けた kind のディレクトリだけを読む。一つも無い多 kind 設定は exit 3。
   `spec` preset の `conform` と `measure` に付ける。
7. **Go の言語バージョンは 1.27**（パッチは `go.mod` に書かない）。

## 帰結

- 上位リポジトリは `SpecPreset()` を土台に kind / edge / rule を足し、`Validate()` の後に
  `yaml.Marshal` して `docdag.yaml` を生成できる。生成物は `docdag lint` / `validate` が読むものと
  同じ契約である。
- CLI の入出力形は変わらない。既存のハーネスはバイナリを差し替えるだけでよい。
- 人が書く kind に `append_only` を付けなければ、多 kind vault でも履歴検査の対象外のままにできる。

## 代替案と棄却理由

- **`pkg/config`**: 短く plan と一致する `config` の方が import 経路が読みやすい。
- **`Severity` を `config` に複製**: `Finding` を lint API で返すなら `model` の公開が避けられない。
- **`Resolve` を安定入口にする**: ファイル探索と Dir 発見は CLI 都合で、生成側は既に組んだ
  `Config` を `Validate` すれば足りる。
- **Marshal をマッピング形のままにする**: 人が読む生成 YAML ではスカラ形の方が既定の意図に合う。
- **多 kind を kind 属性なしに全面許可する**: 人が書く clause まで append-only にすると、正当な
  改稿が違反になる。

## 実装時の注記

- `Load` は Unmarshal のみ。ファイルが `kinds:` とトップレベル `id_width` を同時に書いたときだけ
  意味を持つ検査は `Resolve` 側に残す。
- map キーの YAML 順序は goccy/go-yaml がソートするため、往復の決定性に追加の `MarshalYAML` は
  要らなかった。
