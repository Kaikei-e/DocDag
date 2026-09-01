# 0003: 条項の様相を 5 値で持ち、MAY を強い許可として明示し、許可と禁止の衝突を検出する

## ステータス

Proposed

## 日付

2026-09-02

## コンテキスト

### 現状の条項モデルでは MAY が「書かないこと」と区別できない

ADR-0001 の `spec` preset は条項に `level: MUST | SHOULD | MAY` を持たせる。しかしこのモデルには 3 つの穴がある。

1. **否定形が表現できない。** BCP 14 のキーワードは MUST / MUST NOT / SHOULD / SHOULD NOT / MAY の 5 つで、
   MUST NOT と SHOULD NOT はそれぞれ独立した定義を持つ。`level` だけでは「X してはならない」が書けず、
   本文の散文に埋もれる。
2. **MAY が弱い許可（禁止の不在）と区別できない。** 義務論理では、a が弱く許可されているとは ¬a が義務として
   導出できないことであり、強い許可とは規範体系が a を許可すると**明示的に**述べていることを指す。強い許可は
   禁止の不在から導出されず、明示的な許可規範として定式化される。`level: MAY` の条項をノードとして
   持たなければ、標準の中で MAY は「触れていない」と同じになり、後から追加された SHOULD NOT が黙って
   上書きする。
3. **衝突が見えない。** 「X を MAY する」と「X を SHOULD NOT する」が両方 binding のとき、DocDag は何も言わない。
   RFC 2119 の SHOULD NOT は「特定の状況では当該の振る舞いが許容されるか有用でさえある正当な理由が
   ありうる」と定義されており、MAY との共存は「その特定の状況」が明示されているときにだけ正当である。
   明示されていない共存は矛盾か、少なくとも要レビューである。

### RFC 2119 の MAY が課す相互運用義務

RFC 2119 §5 は、MAY（OPTIONAL）を「真に任意」と定義した上で、オプションを含まない実装は含む実装と
相互運用できるよう準備しなければならず（MUST）、逆も同様である、と定める。つまり MAY 条項は
「任意の機能 X」と「X の有無に関わらず相互運用が成立すること」の対で、後者は MUST 水準の義務である。
uzushio 標準で言えば、「エージェントは推論的 grader を使ってもよい（MAY）」は「推論的 grader を使わない
リポジトリとも適合レポートの形式が互換である（MUST）」を伴う。この対を明示しないと、MAY が増えるほど
標準の互換性が暗黙に壊れる。

### DocDag の現状と前例

- `derived_conflict` は「導出辺と構造辺が矛盾する」を error で報告し、どちらが正しいか推定しない。
- `inverse_mismatch` と `id_collision` は 2 文書間の対検査で、`related` に相手を列挙する。
- ADR-0001 §C-4 で、MUST ＝ strict rule、SHOULD ＝ defeasible rule、逸脱記録 ＝ defeater、と
  defeasible deontic logic（DDL）への対応を採用した。DDL では、明示的な許可規範が反対の義務への例外として
  働き、その計算は線形時間で行える。

### 満たすべき要件

- R1 5 つの様相を一意に表現できる
- R2 MAY 条項が明示ノードとして存在し、`--binding` に含まれる
- R3 同じ主題に対する非両立な様相の組を、両方が binding のときに検出できる
- R4 明示的な例外（lex specialis）を辺として記録でき、記録された衝突は「抑止済み」として区別表示される
- R5 MAY に伴う相互運用義務を辺として記録でき、欠落を警告できる
- R6 DocDag は散文（例外の適用範囲）を解釈しない。記録するだけである
- R7 `adr` preset に影響しない

## 決定

### D1. `level` を `modality` に置き換え、5 値の閉じた語彙にする

```yaml
kinds:
  clause:
    dir: spec/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    closed: true
    fields:
      modality: {one_of: [MUST, MUST_NOT, SHOULD, SHOULD_NOT, MAY], required: true}
```

- `kinds[].fields[].one_of` / `required` を ADR-0001 D1 の `fields:` に追加する（`fields:` は D4 の
  deprecated 用に既にある。語彙の宣言も同じ場所に置く）。違反は `unknown_field_value`（error、構造）。
- ADR-0001 の `effective_must` / `effective_should` 射影は `modality` を読むよう改める。
  `MUST_NOT` は `MUST` と同じく `enforces` 入辺を必要とし、無ければ効力は `SHOULD_NOT` 相当に落ちる。

### D2. 主題（topic）を kind にし、`about` を辺にする

```yaml
kinds:
  topic:
    dir: spec/topics
    id: '^topic/[a-z0-9/-]+$'
edges:
  - name: about
    key: about
    from: [clause]
    to: [topic]
    min_outbound: 1          # 条項は必ず主題を持つ
```

- 主題を文字列属性ではなく文書にするのは、綴りの揺れを `dangling_ref` で捕まえるためである。
  文字列比較では `topic/grader-inferential` と `topic/inferential-grader` が別主題になり、衝突検出が
  偽陰性になる。
- `topic` 文書の本文は主題の定義（1 段落）で、`context` が近傍に表示する。

### D3. 許可・禁止の衝突を構造検査 `modality_conflict` として検出する

同じ topic に `about` 辺を持つ 2 つの条項 A・B が両方 binding（ADR-0001 の `binding:` 射影）であり、
様相の組が下表の × に当たるとき、`modality_conflict` を報告する。

| A \ B | MUST | MUST_NOT | SHOULD | SHOULD_NOT | MAY |
| --- | --- | --- | --- | --- | --- |
| MUST | — | **×（強）** | — | × | — |
| MUST_NOT | **×（強）** | — | × | — | × |
| SHOULD | — | × | — | × | — |
| SHOULD_NOT | × | — | × | — | × |
| MAY | — | × | — | × | — |

- **強い衝突**（MUST × MUST_NOT）は error で、例外辺があっても抑止できない。DDL で strict rule が defeater に
  倒されないのと同じ扱い。
- **弱い衝突**（少なくとも一方が SHOULD / SHOULD_NOT / MAY）は、D4 の `excepts` 辺が両者の間に存在しなければ
  error、存在すれば「抑止された finding」として通常出力から消え、`validate --show-suppressed` で表示する。
- finding は A（ID 順で小さい方）に置き、`related` に B と共有 topic を列挙する。`fix:` は
  `declare excepts: <B> in <A> with scope:, or revise one modality`。
- `modality_conflict` は `id_collision` と同じく対検査で、topic ごとに binding 条項を集めて様相の組を見る。
  計算量は topic あたりの条項数の 2 乗だが、topic は細かく切る前提（1 topic あたり数本）で実用上線形。

### D4. 例外を `excepts` 辺として記録する（defeater）

```yaml
# UZ-G-012（MAY: 推論的 grader を使ってよい）
---
id: UZ-G-012
kind: clause
modality: MAY
about: [topic/grader/inferential]
excepts:
  - {ref: UZ-G-003, scope: "較正記録（ADR-0001 D3 の measures 辺）を持つ場合に限る"}
interop: [UZ-G-001]
---
```

```yaml
edges:
  - name: excepts
    key: excepts
    from: [clause]
    to: [clause]
    acyclic: true
    direction: forward
    attrs:
      scope: {required: true, type: string}
```

- 向きは「例外（より特殊な条項）→ 一般条項」。lex specialis の記録であり、`scope` は散文である。
  DocDag は `scope` の内容を評価しない（R6）。人とエージェントが `context` で読む。
- `excepts` は非循環。A が B を例外とし B が A を例外とする状態は `cycle` になる。
- `excepts` が MUST / MUST_NOT を対象にしていれば `excepts_strict`（error）。strict rule は倒せない。

### D5. 相互運用義務を `interop` 辺として記録し、欠落を警告する

```yaml
edges:
  - name: interop
    key: interop
    from: [clause]
    to: [clause]
rules:
  - name: may_without_interop
    severity: warn
    when:
      attr: {modality: {eq: MAY}, status: {eq: accepted}}
      not_outbound: interop
    message: "is MAY but names no MUST clause that guarantees interoperation without it"
  - name: interop_not_must
    severity: error
    when:
      outbound: interop
      via: {edge: interop, attr: {modality: {not: MUST}}}
    message: "interop must point at a MUST clause"
```

- `interop` の対象は MUST 条項でなければならない（RFC 2119 §5 の「MUST be prepared to interoperate」）。
- `may_without_interop` は warn に留める。相互運用が自明な MAY（例：任意のログ出力）は多く、error にすると
  MAY の乱発を招く逆効果がある。

### D6. 出力への反映

- `query --binding` の既定列に `modality` を加える。
- `context <ref>` は、同じ topic の binding 条項と、`excepts` の両方向、`interop` の対象を「近傍」として
  出す。抑止された `modality_conflict` は `suppressed by excepts UZ-G-012 → UZ-G-003 (scope: …)` と 1 行で示す。
- `stats` に topic ごとの条項数・様相の分布・抑止された衝突数を加える。

### 非目標

- 自由選択許可（P(a ∨ b) から P(a) ∧ P(b) を導く）の意味論
- `scope` の機械的評価、条件付き義務（「Y のとき X せよ」）の条件部の構造化
- 時間による優先（lex posterior）— ADR-0005 の as-of で扱う
- 衝突の自動解決。DocDag は `derived_conflict` と同様、どちらが正しいか推定しない

## 根拠（調査結果・出典）

### A. 規格

- RFC 2119 §3–§5（IETF）：SHOULD NOT は「特定の状況で当該の振る舞いが許容されるか有用でさえある正当な
  理由がありうる」、MAY は「真に任意」で、オプションを含まない実装は含む実装と相互運用できるよう準備
  しなければならず（MUST）、逆も同様。D3 の弱い衝突と D5 の `interop` の根拠。
  https://www.ietf.org/rfc/rfc2119.txt
- RFC 2119 §6：キーワードは相互運用に実際に必要な場合か、害を及ぼしうる振る舞いを制限する場合にのみ
  使う（乱用の戒め）。D5 を warn に留める根拠。
  https://lists.w3.org/Archives/Public/ietf-http-wg/2013JulSep/0551.html （§6 の引用を含む議論）

### B. 義務論理

- Governatori, Olivieri, Rotolo, Scannapieco「Computing Strong and Weak Permissions in Defeasible Logic」
  （J. Philosophical Logic 2013）：弱い（消極的）許可は ¬a が義務として証明できないこと、強い（積極的）
  許可は規範が a の許可を明示的に述べていること。強い許可は禁止の不在から導出されず、明示的な許可規範として
  定式化される。反対の義務への例外として働く明示的許可規範や、権利を符号化する許可規範を扱い、
  計算量は線形。D2・D3・D4 の根拠。 https://arxiv.org/abs/1212.0079
- Defeasible deontic logic の三分法（strict / defeasible / defeater）と constitutive / regulative の区別。
  strict rule は例外なく帰結が従い、defeater は結論を導かず defeasible rule の結論を阻止する。D3 の
  「強い衝突は抑止不能」と D4 の `excepts_strict` の根拠。 https://arxiv.org/abs/2203.16275 §2.2.1
- 強い許可・弱い許可・自由選択許可の概説（SEP「Deontic Logic」および関連文献）：弱い許可は義務の双対、
  強い許可は明示的な許可規範の存在。 https://plato.stanford.edu/entries/logic-deontic/

### C. DocDag 一次情報

- `docs/checks.md`：`derived_conflict` はどちらが誤りか推定しない。`id_collision` / `inverse_mismatch` は
  対検査で `related` に相手を列挙する。D3 の形式の前例。
  https://github.com/Kaikei-e/DocDag/blob/main/docs/checks.md
- `internal/config/config.go` `EdgeSpec.MinOutbound`：`about` の必須化（`min_outbound: 1`）は既存機構で足りる。

## 検討した代替案

- **A. MAY を「禁止の不在」として扱い、ノードにしない。** 不採用。強い許可を記録できず、衝突検出も
  `interop` の記録も不可能になる。後から追加された SHOULD NOT が MAY を黙って上書きする。
- **B. `level` と `polarity`（positive / negative）の 2 フィールドにする。** 不採用。`MAY` に `polarity`
  が意味を持たず（BCP 14 に MAY NOT は無い）、無効な組み合わせを検査する必要が生じる。5 値の閉じた語彙の
  方が誤りようがない。
- **C. `about` を文字列属性にし、属性の一致で衝突を検出する。** 不採用。綴りの揺れが偽陰性になる。
  topic を文書にすれば `dangling_ref` が綴り違いを捕まえ、`context` が主題の定義を表示できる。
- **D. 日付の新しい条項を自動的に優先する（lex posterior）。** 不採用（本 ADR では）。DocDag は
  どちらが正しいか推定しないという方針に反する。時点依存の効力は ADR-0005 で扱う。
- **E. DDL の推論器を内蔵し、`scope` を条件として評価する。** 不採用。式言語を置かないという
  ADR-0001 の判断に反し、`scope` は散文である。

## 影響とトレードオフ

**得るもの**

- 標準の中で MAY が「明示された自由」として一級市民になり、後続の禁止規範との衝突が CI で見える。
- 例外が `excepts` 辺として残るため、「なぜ MAY と SHOULD NOT が共存しているのか」が `context` で読める。
- MAY が増えたときの互換性の暗黙の劣化を `interop` の欠落警告で早期に捕まえられる。

**失うもの・リスク**

- **語彙の増加。** clause kind に `modality` / `about` / `excepts` / `interop` の 4 キー、kind に `topic` が
  増える。`spec` preset のテンプレート（`docdag new --kind clause`）でこれらを雛形に含め、手書きの負担を
  減らす。
- **topic の粒度設計が衝突検出の精度を決める。** 粗すぎれば偽陽性（無関係な条項が同じ topic に集まる）、
  細かすぎれば偽陰性。運用ガイドとして「topic は 1 段落で定義でき、条項が 2〜5 本ぶら下がる粒度」を
  configuration.md に書く。`stats` の topic 分布で監視する。
- **`excepts` の `scope` は散文なので、例外の妥当性は人のレビューに残る。** これは意図した境界であり、
  DocDag が散文を解釈し始める方が危険である。
- **`level` → `modality` の改名は `spec` preset 内の破壊的変更。** `spec` preset は未リリースなので
  影響はないが、ADR-0001 の文書と `effective_*` 射影の定義を本 ADR に合わせて更新する。
- **`modality_conflict` は対検査で、topic あたりの条項数の 2 乗。** 実用上は線形だが、1 topic に 50 本以上
  ぶら下がるような vault では `lint`（ADR-0004）が粒度の粗さを警告する。

## 関連ADR

- 0001（`spec` preset、`binding:` 射影、`fields:`）— 本 ADR は `fields[].one_of` / `required` を追加し、
  `effective_*` 射影の定義を `modality` ベースに改める
- 0002（`target:`）— `interop` の対象が MUST であることは `target: {attr: {modality: {eq: MUST}}}` でも
  書ける。D5 のルール形式と `target:` 形式のどちらに寄せるかは 0002 の実装後に統一する
- 0004（`preset lint`）— topic 粒度の警告と、衝突表の到達不能な組の検出
- 0005（有効期間）— 衝突検出の「両方が binding」を as-of 時点で評価する
