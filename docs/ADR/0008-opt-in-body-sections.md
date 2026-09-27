---
title: "本文の見出し構造を opt-in の sections: で宣言し、missing_section と section_order で検査する"
status: accepted
date: 2026-09-27
depends-on: ["0001"]
---

# 0008: 本文の見出し構造を opt-in の `sections:` で宣言し、`missing_section` と `section_order` で検査する

## コンテキスト

DocDag の検査は frontmatter とファイル名を読む。組み込みの検査およそ 40 件のうち、本文を読むのは
`references.dangling`（本文中の wikilink / Markdown リンクの宛先が存在するか）の 1 件だけである。

そのため「記録が ADR として体を成しているか」は保証されない。frontmatter が正しければ、本文が空でも、
決定の節が無くても、代替案を書いていなくても `validate` は通る。ADR の価値は「何を」より
「なぜ」と「何を捨てたか」にあるのに、そこは人のレビューにしか委ねられていなかった。

テンプレートの見出しは、コーパスによって違う。DocDag 自身の ADR は日本語見出し
（コンテキスト / 決定 / 検討した代替案 …）で書かれている。一方、MADR は
`Context and Problem Statement` / `Decision Outcome`、Nygard 形式は `Context` / `Decision` /
`Consequences` を使う。0006 は途中から `帰結` / `代替案と棄却理由` という別の見出しを使っており、
同じコーパスの中でさえ揺れがある。

## 決定

**本文の見出し構造を、設定で宣言した場合にだけ検査する。** 語彙は 0001 の方針どおり宣言的な
小さな DSL に留め、式言語は入れない。

```yaml
sections:
  required: ["Context", "Decision|Decision Outcome", "Consequences"]
  level: 2            # 省略時はどのレベルの見出しでもよい
  ordered: true       # 省略時は順序を問わない
  when:
    status: [accepted] # 省略時はすべての status に適用する
```

- **置き場所** — トップレベル（単一 kind のコーパス）と `kinds[]` ごと。
- **見出しの照合** — ATX 見出し（`#`〜`######`）を対象に、大文字小文字を無視する。前後の空白と
  末尾の `:` は取り除いて比べる。1 項目の中を `|` で区切ると、そのうちどれか 1 つがあればよい。
  MADR と Nygard、あるいは同じコーパス内の表記揺れを 1 つの宣言で受け止めるための仕組みである。
- **コードブロック内は無視する** — フェンス（```` ``` ```` / `~~~`）の中の `#` 行は見出しとして
  数えない。`references` がコードブロック内のリンクを無視するのと同じ扱いにする。
- **所見** — 必須の見出しが無ければ `missing_section`、`ordered: true` で順序が逆転していれば
  `section_order` を出す。どちらも既定は error で、`structural:` で変更できる。`sections:` を
  宣言していないコーパスでは決して発火しない。
- **設定の検証** — 空の見出し名、1〜6 以外の `level`、重複した見出し名は `ErrInvalidConfig` にする。
- **preset では有効にしない** — `adr` / `spec` のどちらの preset も既定では `sections:` を持たない。
  テンプレートはコーパスごとに違うので、DocDag が 1 つの形を押し付けることはしない。

DocDag 自身のコーパスでは、全記録が持つ見出しだけを必須にする。0006 の表記揺れは `|` で吸収する。

```yaml
sections:
  required: ["コンテキスト", "決定", "検討した代替案|代替案と棄却理由", "影響とトレードオフ|帰結"]
```

## 根拠（調査結果・出典）

- **MADR** はテンプレートの節を固定している（Context and Problem Statement / Decision Drivers /
  Considered Options / Decision Outcome …）。<https://adr.github.io/madr/>
- **markdownlint** のカスタムルールや **Vale** の `existence` / `sequence` ルールは、見出しトークンに
  対して「必須の節があるか」「順序が正しいか」を検査する。どちらも見出しという構造単位で判定しており、
  文章の意味には踏み込まない。
  <https://github.com/DavidAnson/markdownlint/blob/main/doc/CustomRules.md> /
  <https://vale.sh/docs/topics/rules/>
- **実測** — DocDag の既存 6 件は、見出しがすべて揃う組が「コンテキスト / 決定」だけだった。0006 だけが
  `帰結` / `代替案と棄却理由` を使っているため、`|` による別名が無いと必須にできる見出しは 2 つに減る。

## 検討した代替案

- **本文の文章と frontmatter の関係の整合を検査する** — 例えば、本文に「000533 を supersede する」と
  書いてあるのに frontmatter に `supersedes` が無い、という食い違いの検出。Alt には実例があるが、判定は
  言語に依存した文章解析になり、「supersede しない」と書いた文を拾う誤検出が避けられない。
  見出しという構造だけを見る本 ADR の範囲に留めた。
- **preset に ADR の見出しを既定で入れる** — 利用リポジトリ（Alt / Plecto / MatrixWhale / modal-one）の
  テンプレートはそれぞれ違う。既定で有効にすると、アップグレードした日に全記録が error になる。
- **見出しの正規表現を書かせる** — 表現力は上がるが、0004 の `lint` が語彙を有限集合として
  扱えなくなる。`|` による列挙で足りない実例は、今のところ無い。
- **見出しの下の本文が空でないことまで検査する** — 「空の節」をどう定義するか（空行だけ、
  `TBD` だけ、など）で規則が増える。まずは見出しの有無と順序に留める。

## 影響とトレードオフ

- **得るもの** — 「決定」「代替案」の節を持たない記録が CI を通らなくなる。ADR の体裁の一部が、
  レビューではなく機械で保証される。
- **見出しの有無しか保証しない** — 節の中身が空でも、見当違いでも通る。本 ADR が保証するのは
  構造であって、内容の質ではない。
- **既存コーパスへの導入コスト** — 過去の記録の見出しが揃っていないコーパスでは、`|` で揺れを
  吸収するか、`when.status` で対象を絞る必要がある。
- **ファイルの再読み込み** — 検査はグラフ構築後に本文を読み直す。Alt 規模（約 1,000 件）でも
  体感できる差は無かった。

## 関連ADR

- [[0001]] — 語彙を宣言的な小さな DSL に留め、式言語を入れないという方針。`sections:` もその範囲で設計した。
- [[0004]] — `sections:` の宣言が空虚かどうか（例えば `when.status` に存在しない status を書いた場合）の
  検査は、今後 `lint` に加える余地がある。
- [[0007]] — 同じリリースで行った fail-closed 化。
