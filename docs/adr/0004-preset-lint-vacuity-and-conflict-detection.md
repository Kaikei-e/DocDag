# 0004: `docdag lint` を追加し、ルール・射影・辺仕様の矛盾・空虚性・不発を 3 層で検出する

## ステータス

Proposed

## 日付

2026-09-02

## コンテキスト

### 「センサーが鳴らない」の二義性

検査が一度も発火しないとき、それは corpus が健全だからか、検査が構造的に発火しえないからか区別できない。
Böckeler（martinfowler.com、2026-04）はコーディングエージェントのハーネスについて、センサーが鳴らないのは
高品質の証拠なのか検知不足なのか判別できず、コードカバレッジやミューテーションテストに相当する
「ハーネスのカバレッジと品質」を測る手段が必要だと述べている。uzushio 標準はこの問題に対して
「機械検査の存在で MUST の効力が決まる」（ADR-0001 R6）と答えるが、その機械検査自身が空虚なら根拠が消える。

同じ問題は形式検証で vacuity（空虚な充足）として知られている。Beer らは商用の検証で、仕様の約 20% が
空虚に成立していた（前件が決して真にならない等）と報告し、Kupferman と Vardi は空虚性の検出と
「興味深い証人（interesting witness）」の生成を定式化した。Fisman らはモデルに依存しない
「本質的空虚性（inherent vacuity）」とモデル依存の空虚性を区別している。

ルールベースの検証でも同型の分類がある。Preece と Shinghal の枠組みでは、異常は冗長性（論理的に包含される
ルール、現実の状況では発火しえないルール、使える結論を導かないルール）、衝突（妥当な入力から非両立な
情報が導かれる）、循環、欠損（妥当な入力に対して結論が出ない）の 4 類型で、下位分類として
unfirable rule、unsatisfiable condition、subsumed rule pair、ambivalent rule pair 等が挙げられる。

### DocDag の現状

- `Config.Validate` は語彙の外側の誤り（未宣言の辺名、`structural:` の未知名・引き下げ、`references` の
  モード違反、`attr` 条件のオペランド数）を設定エラー（exit 3）で弾く。
- しかし語彙の**内側**の誤りは検査しない。`attr: {status: {eq: accepted}}` と `attr: {status: {eq: proposed}}`
  を AND した条件、`max_inbound: 1` の辺に `inbound: {edge: X, min: 5}`、`status_values` に無い値との `eq`、
  `from: [conform]` の辺に対する `inbound` を `clause` 以外の kind に課すルール、いずれも受理される。
- ルールが corpus で一度も発火していないことを知る手段がない（`stats` は辺と系譜の統計のみ）。
- リポジトリは失敗モードごとの fixture corpus（`testdata/fixtures/*`）を持つが、これは DocDag 自身の
  テストであり、ユーザーが `docdag.yaml` に書いた preset のルールにはテストの仕組みがない。
- ADR-0001〜0003 で `projections:`、`min`/`max`、`via`、`target:`、`path_constraints:`、`modality_conflict`
  が加わり、語彙の内側で矛盾を作る余地が増える。

### 先行事例：ルールにはテストが付く

Semgrep はルールごとにテストファイルを置き、`ruleid: <id>` で「ここで発火すべき」（偽陰性の防止）、
`ok: <id>` で「ここでは発火してはならない」（偽陽性の防止）を注釈し、`--test` で検証する。
ルール作成手順として「少なくとも 1 つの真陽性と 1 つの真陰性を書く」ことが推奨されている。

### 満たすべき要件

- R1 設定だけから判る矛盾・空虚性を、corpus なしで検出する（本質的空虚性）
- R2 corpus に対して一度も発火しないルール・射影を検出する（モデル依存の空虚性）
- R3 ルールごとに真陽性・真陰性の fixture を要求し、機械的に検証できる
- R4 検出は決定的で、SAT/SMT ソルバ等の外部依存を持たない
- R5 lint の結果は `validate` の合否に影響しない（設定の健全性と文書の合否はライフサイクルが違う）
- R6 finding は `docdag.yaml` の該当行に位置し、`fix:` を出す
- R7 式言語を導入しない（ADR-0001 と同じ境界）

## 決定

### D1. `docdag lint` コマンドを追加し、3 層で検査する

```
docdag lint                          # 層 1: 設定のみ（本質的空虚性・矛盾）
docdag lint --corpus                 # 層 2: 現行 vault に対する不発・恒真
docdag lint --fixtures lint/         # 層 3: ルールごとの真陽性・真陰性 fixture
docdag lint --all                    # 全部
```

出力形式は `validate` と同じ（text / json / github / rdjson）。finding の `path` は `docdag.yaml`（preset
同梱ルールの場合は `<preset:adr>` の仮想パス）、`line` はルール名の行。exit code は 0（finding なし）、
1（error あり）、2（warn のみ、`--strict` で 1）、3（設定不正）。`validate` は lint を実行しない（R5）。

### D2. 層 1：設定のみで検出する異常

すべて `Condition` の DNF 展開と、有限領域の整合性判定で行う。SAT ソルバは使わない（R4）。
`any_of` を選言、それ以外を連言として DNF に展開し、各連言について次を判定する。

| finding | severity | 判定 | 対応する古典的異常 |
| --- | --- | --- | --- |
| `unsatisfiable_condition` | error | 同一 attr に `eq: X` と `eq: Y`（X≠Y）、`eq: X` と `not: X`、`status_values` / `one_of` に無い値との `eq`、`inbound: E` と `not_inbound: E`、`min > max`、`min` が辺の `max_inbound` を超える、`from`/`to` の kind 制約と `attr: {kind: ...}` の矛盾、`via` / `target` の入れ子条件が同様に矛盾 | unsatisfiable condition |
| `unfirable_rule` | error | ルールの全連言が `unsatisfiable_condition` | unfirable rule |
| `tautological_rule` | warn | 条件が空、または全 attr 条件が語彙全体を覆う（`any_of` で `status_values` を尽くす等）＝全文書で発火 | （vacuity: 前件が常に真） |
| `subsumed_rule` | warn | ルール A の条件が B の条件を含意し（DNF の各連言が B のいずれかの連言のリテラル集合を包含）、severity が同じか A が弱い | subsumed rule pair |
| `shadowed_rule` | warn | A ⊆ B で A の severity が B より強い（B が warn で A が error なら、A の発火は常に B も発火させるが逆はない。表示上の重複） | redundant rule |
| `ambivalent_fix` | error | 2 つのルールの `fix:` が同じ文書の同じキーに相反する値を要求する（`status: superseded` と `status: withdrawn` 等）。`fix:` テンプレートの静的解析で判定 | ambivalent rule pair |
| `unused_edge` | warn | `edges:` に宣言されているが、いずれのルール・射影・`target` / `path_constraints` / `inverse` / `derived_edges` からも参照されない辺 | deficiency: unused input |
| `unused_status` | warn | `status_values` にあるが、いずれのルール・射影からも参照されない値 | deficiency: unused input |
| `unsatisfiable_projection` / `tautological_projection` | error / warn | `projections:` に同じ判定を適用 | — |
| `projection_cycle` | error（設定エラー相当、exit 3） | 射影が自分自身を参照する | circularity |
| `prefer_target` | warn | `path_constraints:` で書かれた制約が `target:` で表現可能（ADR-0002 D3） | redundancy |

判定の根拠は、属性の領域が有限（`status_values`、`one_of`、`kinds`）で、辺の次数条件が整数区間で、
様相の深さが 2 に固定されている（ADR-0001・0002）ことである。各連言はリテラルの有限集合なので、
整合性は集合の走査で決まる。含意（subsumption）はリテラル集合の包含に帰着する。
これは Preece らが「組合せ爆発」を懸念した一般のルールベース検証より遥かに狭い問題で、DocDag が
語彙を固定して式言語を置かないという判断の直接の見返りである。

### D3. 層 2：corpus に対する不発・恒真

```
docdag.yaml:41: WARN never_fired deviation_pressure: fired on 0 of 128 clause documents
  fix: keep it only if a fixture under lint/ shows it can fire (docdag lint --fixtures)
docdag.yaml:23: WARN always_fired no_counterexample: fired on 128 of 128 clause documents
```

- `never_fired`（warn）：ルールがどの文書でも発火しない。`unfirable_rule` と異なり、設定だけでは
  判らない（corpus 依存）。fixture が存在して真陽性を通していれば `never_fired` は info に落ちる
  （「発火しうるが今は健全」）。これが Böckeler の二義性への回答で、**fixture が「鳴りうる」ことを、
  corpus が「今は鳴っていない」ことを、それぞれ証明する**。
- `always_fired`（warn）：全対象文書で発火する。ルールが実質的に恒真か、corpus 全体が違反している。
- `never_true` / `always_true`（warn）：射影に同じ判定。`binding:` に指定された射影が `never_true` なら error
  （binding 集合が空になる設定は壊れている）。
- `unused_edge_in_corpus`（info）：宣言された辺型が corpus に 1 本もない。
- 対象文書数は kind で絞る（`from`/`to`、`attr: {kind: ...}` から静的に決まる範囲）。
- `--since <rev>` を付けると、基準リビジョンの corpus でも評価し、「基準以降に初めて発火した／しなくなった」
  ルールを `newly_fired` / `stopped_firing`（info）として出す。`internal/vcs` の `File(rev, path)` を使い、
  `--immutable-since` と同じ機構で基準側のグラフを構築する。

### D4. 層 3：ルールごとの fixture

```
lint/
  orphan_must/
    ruleid/                     # ここでは orphan_must が発火しなければならない
      UZ-V-900.md
      ...
    ok/                         # ここでは発火してはならない
      UZ-V-901.md
      conform-901.md
  stale_target/
    ruleid/ ...
    ok/ ...
```

- ルール名（`rules[].name`、`projections[].name`、構造検査名）ごとにディレクトリを切り、`ruleid/` と `ok/`
  を置く。各ディレクトリは独立した小さな corpus として、本体と同じ `docdag.yaml` で評価する。
- `missing_fixture`（warn）：`rules:` にあるルールに `ruleid/` または `ok/` が無い。preset 同梱ルールは
  DocDag が fixture を同梱するので対象外。
- `fixture_mismatch`（error）：`ruleid/` で発火しない、または `ok/` で発火する。finding は fixture の
  文書に置き、`related` に `docdag.yaml` のルール行を添える。
- 命名は Semgrep の `ruleid:` / `ok:` に合わせる。エージェントにとって既知の慣習であることを優先した。
- `docdag new --fixture <rule>` で雛形を生成する（`ruleid/` に発火する最小文書、`ok/` に発火しない最小文書。
  最小文書は層 1 の DNF から機械生成できる：連言のリテラルを満たす文書と、1 リテラルだけ外した文書）。

### D5. `validate` との関係、CI への組み込み

- `validate` は lint を呼ばない。lint は `docdag.yaml` または `lint/` が変更された PR、および定期実行で走らせる。
  `docs/ci.md` に「`paths: [docdag.yaml, lint/**]` で lint、それ以外で validate」の例を載せる。
- pre-commit hook と Claude Code プラグインの PostToolUse フックは、`docdag.yaml` を触った編集に限って
  `lint`（層 1）を追加実行する。層 1 は corpus を読まないので数 ms で終わる。
- `--format json` に `schema_version` を持たせ、`validate` の JSON とは別のトップレベル種別（`"kind": "lint"`）にする。

### D6. `spec` preset の同梱 fixture

ADR-0001 の `spec` preset に同梱するルール（`orphan_must`、`orphan_test`、`stale_premise`、
`deviation_pressure`、`no_counterexample`）、ADR-0002 の `stale_target`、ADR-0003 の `modality_conflict` /
`may_without_interop` / `interop_not_must` / `excepts_strict` について、`ruleid/` と `ok/` を DocDag
リポジトリの `testdata/lint/spec/` に同梱し、DocDag 自身の CI で `lint --fixtures` を回す。

### 非目標

- 汎用 SAT/SMT による充足可能性判定（本 ADR の判定は有限領域の集合演算で閉じる）
- 散文（`message`、`scope`、本文）の検査
- `validate` への統合、lint の結果による `validate` の合否変更
- 歴史全体を走査する「最後に発火したコミット」の探索（`--since` で 2 時点比較に留める。必要なら後続 ADR）

## 根拠（調査結果・出典）

### A. 動機

- Böckeler「Harness engineering for coding agent users」（martinfowler.com、2026-04-02）：センサーが一度も
  鳴らないのは高品質の証拠か検知不足か判別できず、ハーネスのカバレッジと品質を測る手段が要る。
  https://martinfowler.com/articles/harness-engineering.html
- ADR-0001 R6・§C-3：MUST の効力は機械検査の存在で決まる。検査自身が空虚なら根拠が消える。

### B. 空虚性検出（形式検証）

- Kupferman, Vardi「Vacuity detection in temporal model checking」（STTT 4(2), 2003）：モデル検査が成功した
  場合にも仕様やモデルの誤りを疑うべきで、Beer らの空虚な充足の検出と興味深い証人の生成を一般化。
  https://link.springer.com/article/10.1007/s100090100062
- 同論文の要約（Academia）：典型的に仕様の約 20% が空虚に成立し、証人式（witness formula）で
  各部分式が仕様の真偽に影響するかを確認する。興味深い証人の生成は PSPACE 完全。
  https://www.academia.edu/100197805/Vacuity_detection_in_temporal_model_checking
- Beer, Ben-David, Eisner, Rodeh「Efficient Detection of Vacuity in Temporal Model Checking」
  （FMSD 18(2), 2001）：論理を変えずに空虚性を定式化し検出する。興味深い証人の初出。
  https://link.springer.com/article/10.1023/A:1008779610539
- Fisman, Kupferman, Sheinvald-Faragy, Vardi「A framework for inherent vacuity」（HVC 2008、
  「Vacuity in practice」の参考文献経由）：モデルに依存しない本質的空虚性の枠組み。層 1 と層 2 の区別の根拠。
  https://link.springer.com/article/10.1007/s10703-014-0221-0

### C. ルールベース検証の異常分類

- Preece, Shinghal「Foundation and application of knowledge base verification」（Int. J. Intelligent Systems
  9(8), 1994）：冗長・矛盾・欠損の知識は誤りの徴候で、異常検出は知識ベース検証の確立した方法。理論的基礎と
  計算上の限界、実用上の有用性を分析。 https://onlinelibrary.wiley.com/doi/abs/10.1002/int.4550090804
- Preece, Shinghal, Batarekh「Principles and practice in verifying rule-based systems」（KER 7(2), 1992）：
  異常を redundancy / ambivalence / circularity / deficiency の 4 類型として一階論理の意味論で定義。
  https://www.cambridge.org/core/journals/knowledge-engineering-review/article/abs/principles-and-practice-in-verifying-rulebased-systems/C342350D35C6F26ED49DF3EACDBDA341
- 同分類の下位項目（unfirable rule、unsatisfiable condition、subsumed rule pair、ambivalent rule pair、
  unused input、unusable consequent）と、自動検査が組合せ爆発に直面するという指摘。
  https://ceur-ws.org/Vol-241/paper2.pdf （Fig. 1）
- COVER が検出する異常の説明（論理的に包含されるルール、現実の状況で発火しえないルール、使える結論を
  導かないルール、衝突、循環、欠損）。 https://users.cs.cf.ac.uk/A.D.Preece/publications/download/mkm2001.pdf

### D. ルールにテストを付ける慣習

- Semgrep「Test rules」：`ruleid: <rule-id>`（偽陰性の防止）、`ok: <rule-id>`（偽陽性の防止）、
  `todoruleid` / `todook`、`--test`。ルールと同名のテストファイルを探す。
  https://semgrep.dev/docs/writing-rules/testing-rules
- Semgrep Editor：ルールをテストするには真陽性を 1 つ以上、真陰性を 1 つ以上書く。
  https://semgrep.dev/docs/semgrep-code/editor

### E. DocDag 一次情報

- `internal/config/config.go` `Validate` / `validateEdges` / `validateRules` / `structuralSeverities`：
  語彙の外側の誤りは既に exit 3 で弾かれる。本 ADR は語彙の内側を対象にする。
- `docs/checks.md`「The fixture corpora」：失敗モードごとの fixture corpus を `testdata/fixtures` に持つ。
  層 3 はこの慣習をユーザー定義ルールに拡張する。 https://github.com/Kaikei-e/DocDag/blob/main/docs/checks.md
- `internal/vcs/vcs.go` `File(rev, path)` / `Changes(base, dir)`：`--since` の 2 時点比較に再利用する。

## 検討した代替案

- **A. SAT/SMT ソルバ（Z3 等）を組み込む。** 不採用。依存が重く、判定は速いがメッセージの決定性と
  `fix:` の生成がソルバのモデルに依存する。有限領域・深さ 2 の連言なら集合演算で十分（D2）。
- **B. 層 2（corpus 依存）だけを実装する。** 不採用。「今は鳴っていない」と「鳴りえない」を区別できず、
  Böckeler の二義性がそのまま残る。
- **C. lint を `validate` に統合する。** 不採用。`validate` は文書の CI ゲートで、設定の健全性は
  `docdag.yaml` が変わったときに見るものである。統合すると全 PR で lint の warn が出続け、無視される。
- **D. DocDag リポジトリのユニットテストに頼る。** 不採用。ユーザーは `docdag.yaml` に独自ルールを書くため、
  DocDag 側のテストでは覆えない。
- **E. fixture の代わりに `message` に「発火例」を散文で書く。** 不採用。散文は検証できない。
  fixture は実行される検査であり、「実行される検査だけが腐らない」に沿う。
- **F. 歴史全体を走査して「最後に発火したコミット」を出す。** 本 ADR では不採用。2 時点比較（`--since`）で
  実用上足り、全走査はコミット数に比例して遅い。必要になれば後続 ADR。

## 影響とトレードオフ

**得るもの**

- 「MUST の効力は機械検査の存在で決まる」の機械検査自身が、空虚でないことを機械で示せる。
- ルールを足すときに fixture が必須になり、ルールの意図が実行可能な形で残る。エージェントがルールを
  追加・改訂する際にも、`ruleid/` と `ok/` が仕様の代わりになる。
- ADR-0001〜0003 で増えた語彙（射影・`target`・`path_constraints`・衝突表）の内側の誤りが設定変更時に捕まる。

**失うもの・リスク**

- **fixture の保守コスト。** ルール 1 本あたり最低 2 文書が増える。`docdag new --fixture` で機械生成し、
  最小文書に留めることで抑える。`missing_fixture` は warn に留め、段階導入できるようにする。
- **`subsumed_rule` / `shadowed_rule` の偽陽性。** 意図的に重ねたルール（粗い warn と細かい error）が
  警告される。`rules[].lint: {allow: [subsumed]}` の抑止指定を認めるが、抑止した事実は `stats` に出す。
- **DNF 展開の指数爆発。** `any_of` の入れ子が深いと連言数が増える。深さ 2・選言肢数の実用的上限
  （8 程度）で線形に近く、超えたら `lint` 自身が `condition_too_wide`（warn）を出して展開を打ち切る。
- **層 2 の結果は corpus に依存し、時間とともに変わる。** これは意図した性質で、`never_fired` を
  「今の corpus の事実」として `stats` に記録し、判断は人が行う。
- **新コマンドと新 JSON 種別の追加。** commands.md、ci.md、agents.md（フックの分岐）を更新する。

## 関連ADR

- 0001（`projections:`、`min`/`max`、`via`、`binding:`）— 層 1・層 2 の検査対象
- 0002（`target:`、`path_constraints:`）— `prefer_target` と、対象条件の空虚性検査
- 0003（`modality_conflict`、`excepts`、`interop`）— 衝突表の到達不能な組と、topic 粒度（1 topic あたりの
  条項数が閾値を超える `topic_too_wide` warn）の検査
- 0005（有効期間）— 層 2 の評価は as-of 時点で行い、`--as-of` を `lint --corpus` にも通す
