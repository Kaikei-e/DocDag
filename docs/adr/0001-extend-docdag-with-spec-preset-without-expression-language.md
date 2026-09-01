# 0001: DocDag を規範文書向け `spec` preset で拡張する（式言語は導入しない）

## ステータス

Proposed

## 日付

2026-09-01

## コンテキスト

### DocDag の現状（v0.2.0、2026-08-27）

DocDag は YAML frontmatter 付き Markdown の集合から型付き有向グラフを抽出し、DAG 不変量を強制し、
問い合わせに答える CLI である。現行モデルの要点は次の通り。

- **制約層と参照層の分離。** frontmatter で宣言された型付き辺（`supersedes:`、`depends-on:`）と
  `derived_edges` で導出した辺だけが不変量・非循環性・ルールの対象。本文中の `[[wikilink]]` 等は
  参照層で、設定が求めない限り検証されない。
- **同一性は数字の連なり。** `339` / `ADR-339` / `000339` は同じノード。ID の正規化は preset の責務で、
  エンジンは ID を比較するだけ（`internal/model/model.go` の `ID` 型のコメント）。
- **status はグラフの射影。** binding ＝ `accepted` かつ何にも supersede されていない。ただし実装上は
  `graph.Binding` / `graph.BindingSet` が `config.EdgeSupersedes` と `isAccepted` を直接参照しており、
  射影の定義は preset ではなくコードに固定されている。
- **ルール語彙は固定かつ完全。** `inbound` / `not_inbound` / `outbound` / `not_outbound`、
  `attr: {eq|not|contains|not_contains|subset_of}`、`any_of`、`not`。式言語は無い
  （`docs/configuration.md`「The rule vocabulary」）。
- **preset は単なる `Config` 値。** `docdag.yaml` は preset の上にマージされ、`edges:` / `rules:` /
  `derived_edges:` は書くとリスト丸ごと置換される。
- **単一 kind・単一ディレクトリ。** `dir` は 1 つ、`status_values` も 1 組。ADR 以外の文書種は想定外。
- **エージェント向け機能は既にある。** `context <ref>`、`--fields`、`validate --touching`、Claude Code
  プラグインの PostToolUse フック。

同梱 preset は `adr` の 1 つで、Alt（985 ADR / 56 PM）と PlectoProxy（96+ ADR）の vault で運用中。

### 新しい用途：uzushio 標準の条項グラフ

uzushio は「12-factor-agents 相当の原則 ＋ AI 自身の評価基盤」を 1 本にまとめた新標準（仕様書＋参照実装
CLI）で、Alt と PlectoProxy に適用する。標準の条項を DocDag で管理したい。条項グラフに必要な性質は
次の 7 点で、いずれも標準の「教条主義を避ける」方針から導かれる（前提の検討経緯は根拠 §B・§C）。

- R1 **導出値を書かない。** `binding`、有効な要求レベル（effective level）、鮮度は frontmatter に手書き
  せず射影で求める。手書きした派生値は preset 改訂で腐る。
- R2 **語彙はデータ。** 辺型・kind・status・フィールドの追加削除は preset の改訂として記録し、各文書を
  書き直さない。
- R3 **複数 kind と非数値 ID。** clause / conform（適合テスト）/ deviation（逸脱記録）/ measure（計測）/
  premise（前提）/ principle / pm が 1 つの vault に混在し、`UZ-V-001` のような接頭辞付き ID を使う。
- R4 **辺に属性。** `supersedes` に理由型（recurrence / premise-collapse / conflict / vocabulary）、
  `deviates-from` に期限、`measures` に一致率とモデル名。
- R5 **辺は機械が生成する側に置く。** 適合テストが `enforces: [UZ-V-001]` を宣言し、計測は CLI が生成する
  文書として vault に置く。条項側に `enforced-by` / `measured-by` を手書きしない。
- R6 **MUST の効力は機械検査の存在で決まる。** `level: MUST` は人の主張で、`enforces` の入辺が無ければ
  効力は SHOULD 相当に落ちる（effective level）。（0003 により `level` は `modality` に改称。
  `MUST_NOT` も同じく enforces 入辺を要し、無ければ SHOULD_NOT 相当に落ちる。）
- R7 **preset 改訂の安全性を機械判定できる。** 新旧設定で `validate` を走らせ findings を比較する。

### 制約

- 「式言語を置かない」「検査は決定的で `fix:` を出せる」という DocDag の設計判断は維持する。
- `adr` preset の既定挙動と MADR 互換を壊さない（Alt / Plecto の CI が依存）。
- frontmatter は YAML のまま（既存 vault と Obsidian 互換、エージェントの読み書き安定性）。
- 言語非依存。Alt は Go/Rust/TS/Python/F# の 20+ サービス、Plecto は Rust。契約はファイルと CLI 出力。

## 決定

DocDag を、単一 preset の ADR ツールから「複数 kind の規範文書グラフ」を扱えるツールへ、以下の 5 つの
最小拡張で広げる。5 つは同じ目的（R1〜R7）に対する不可分の変更として 1 つの決定にまとめる。
式言語・外部スキーマ言語・Datalog エンジンは導入しない。

### D1. `kinds:` — 複数 kind と kind ごとの同一性

```yaml
kinds:
  clause:    {dir: spec/clauses,     id: '^UZ-[A-Z]-\d{3}$', status_values: [proposed, trial, accepted, superseded, withdrawn], closed: true}
  conform:   {dir: spec/conform,     id: '^conform/[a-z0-9-]+$'}
  deviation: {dir: spec/deviations,  id: '^dev-\d{4}$', closed: true}
  measure:   {dir: spec/measures,    id: '^interp/UZ-[A-Z]-\d{3}@\d{4}-\d{2}-\d{2}$'}
  premise:   {dir: spec/premises,    id: '^premise/[a-z0-9/-]+$'}
```

- `kinds:` を書かない設定は従来通り単一 kind（`dir` / `id_width` / `status_values` がその kind の定義）。
  `adr` preset は変更しない。
- kind は `kind:` フィールドで明示するか、`dir` から推定する。両方あって食い違えば `kind_mismatch`（error）。
- ID 正規化は kind ごとの `id` パターンで行う。エンジンは今まで通り正規化済み ID を比較するだけ。
  `id` を省略した kind は現行の数字連なり規則を使う。
- `closed: true` の kind では preset が知らない frontmatter キーを `unknown_field`（error）にする。
  既定は open（現行通り無視）。
- `edges[].from` / `edges[].to` で kind を制約し、違反は `edge_kind_mismatch`（error）。

### D2. 辺の属性

```yaml
edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    attrs:
      reason: {required: true, one_of: [recurrence, premise-collapse, conflict, vocabulary]}
  - name: deviates-from
    key: deviates-from
    from: [deviation]
    to: [clause]
    attrs:
      expires: {required: true, type: date}
  - name: measures
    key: measures
    from: [measure]
    to: [clause]
    attrs:
      agreement: {required: true, type: number}
      model:     {required: true, type: string}
```

- 辺キーのリスト要素は、従来のスカラー参照に加えて `{ref: <id>, <attr>: <value>, ...}` のマップを受け
  付ける。`ref` 以外のキーが `attrs` に無ければ `edge_attr_unknown`（error）、`required` が欠ければ
  `edge_attr_missing`（error）、型・`one_of` 違反は `edge_attr_invalid`（error）。
- `attrs` を宣言しない辺は従来通りスカラー参照のみ。マップ要素は `invalid_ref` のまま（挙動不変）。

### D3. `projections:` — 固定語彙による導出属性、`binding` の preset 化

```yaml
# （0003 により改訂）`level` は `modality` に、`effective_must` は MUST と MUST_NOT の
# any_of に、`binding:` は MAY を含む `effective` になった。実装は configuration.md を参照。
projections:
  - name: enforced
    when: {inbound: enforces}
  - name: effective_must
    any_of:
      - when: {attr: {modality: {eq: MUST},     status: {eq: accepted}}, inbound: enforces, not_inbound: supersedes}
      - when: {attr: {modality: {eq: MUST_NOT}, status: {eq: accepted}}, inbound: enforces, not_inbound: supersedes}
  - name: effective_should
    any_of:
      - when: {attr: {modality: {eq: SHOULD},     status: {eq: accepted}}, not_inbound: supersedes}
      - when: {attr: {modality: {eq: SHOULD_NOT}, status: {eq: accepted}}, not_inbound: supersedes}
      - when: {attr: {modality: {eq: MUST},       status: {eq: accepted}}, not_inbound: enforces, not: {inbound: supersedes}}
      - when: {attr: {modality: {eq: MUST_NOT},   status: {eq: accepted}}, not_inbound: enforces, not: {inbound: supersedes}}

binding: effective        # query --binding が返す集合。省略時は preset 既定
```

- `projections[].when` は `rules[].when` と同じ `Condition` を使う。導出結果は真偽値の仮想属性で、
  `rules` / 他の `projections` / `query --fields` / `context` から `attr` として参照できる。
  射影は非循環でなければならず、循環は設定エラー（exit 3）。
- `binding:` で「`query --binding` が返す集合」を preset が定義する。`adr` preset には
  `binding: accepted_unsuperseded`（`attr: {status: {eq: accepted}}, not_inbound: supersedes`）を
  射影として同梱し、`graph.Binding` / `BindingSet` はその射影を評価する実装に置き換える。
  出力は現行と同一。
- `Condition` に 2 つの語彙を足す。どちらも「固定かつ完全」の性質を保つ（根拠 §C-3）。
  - **次数の閾値**: `inbound: {edge: deviates-from, min: 5}` / `max`。既存の文字列形は `min: 1` の糖衣。
  - **一段隣の属性**: `via: {edge: premise, attr: {status: {eq: retired}}}`（出辺先のいずれかが条件を
    満たす）。入辺側は `via_inbound`。ネストは 1 段に限定する（推移閉包は `resolve` の領分）。
- 式言語は導入しない。算術・文字列操作・変数束縛は語彙に含めない。

### D4. `fields:` — フィールドの生命周期と preset 版

```yaml
preset_version: 3
fields:
  owner: {deprecated: true, since: 2, migrate_to: "owned-by"}   # 辺 owned-by へ
```

- `deprecated` なフィールドの使用は `deprecated_field`（warn）。`sunset:` の日付を過ぎれば error。
- `stats --fields` でフィールドごとの使用数と最終更新（git 履歴由来）を出す。削除判断は数字で行う。
- `preset_version` を `validate --format json`、`query --fields`、`context` の出力ヘッダに含める。
  リポジトリ側の manifest が準拠 preset 版を固定するために使う。
- 改訂の安全性判定（R7）は新機能を足さず、手順として文書化する：
  `docdag validate --config new.yaml --format json` と旧設定の出力を diff し、findings 集合が不変なら
  互換（minor）、変化すれば非互換（major）。将来 `validate --baseline-config` として糖衣化してよい。

### D5. 生成文書と辺の置き場所（運用規約）

コード変更ではなく `spec` preset のドキュメントに規約として書く。

- 辺は変化の少ない側ではなく、機械が生成する側に宣言する。`conform` 文書が `enforces:` を持ち、
  `measure` 文書が `measures:` を持つ。`clause` は `premise` / `rationale` / `counterexample` だけを持つ。
- `measure` 文書は uzushio の CLI が生成する。手書きしない。鮮度は `updated` のようなフィールドではなく
  ファイルの存在と git 履歴から導出する。
- 適合テスト実体（`test.sh` 等）は Markdown ではないので、`conform/<name>.md` の薄い frontmatter 文書が
  実体へのパスを持つ。`derived_edges` で TOML から辺を拾う拡張は行わない（YAML frontmatter 一本を守る）。

### 非目標（この ADR では決めない）

- パス等式（可換図式）の検査 — 後続 ADR
- MAY 条項のノード化と SHOULD NOT との衝突検出 — 後続 ADR
- ルールの空虚性・充足可能性の lint（`preset lint`）— 後続 ADR
- 有効期間（in-force 区間）からの status 射影 — 後続 ADR
- 式言語、CUE / Nickel 等の外部スキーマ言語、Datalog エンジンの内蔵 — 採用しない（代替案参照）

### `spec` preset の骨子（同梱候補）

```yaml
preset: spec
preset_version: 1
kinds: { clause: {...}, conform: {...}, deviation: {...}, measure: {...}, premise: {...}, principle: {...}, pm: {...}, topic: {...} }   # topic は 0003 の追加
edges:
  - {name: supersedes,     key: supersedes,     from: [clause, premise], to: [clause, premise], acyclic: true, direction: forward, attrs: {reason: {required: true, one_of: [recurrence, premise-collapse, conflict, vocabulary]}}}
  - {name: enforces,       key: enforces,       from: [conform],   to: [clause]}
  - {name: deviates-from,  key: deviates-from,  from: [deviation], to: [clause], attrs: {expires: {required: true, type: date}}}
  - {name: premise,        key: premise,        from: [clause],    to: [premise]}
  - {name: rationale,      key: rationale,      from: [clause],    to: [principle]}
  - {name: counterexample, key: counterexample, from: [clause, principle], to: [pm]}
  - {name: measures,       key: measures,       from: [measure],   to: [clause], attrs: {agreement: {required: true, type: number}, model: {required: true, type: string}}}
  # 以下 3 辺と clause の `fields: {modality: {one_of: [...], required: true}}` は 0003 の追加
  - {name: about,          key: about,          from: [clause],    to: [topic],  min_outbound: 1}
  - {name: excepts,        key: excepts,        from: [clause],    to: [clause], acyclic: true, direction: forward, attrs: {scope: {required: true, type: string}}}
  - {name: interop,        key: interop,        from: [clause],    to: [clause]}
projections:
  - {name: enforced,       when: {inbound: enforces}}
  # （0003 により改訂）MUST_NOT は MUST と同じく enforces 入辺を要し、binding は MAY を含む effective
  - {name: effective_must, any_of: [{when: {attr: {modality: {eq: MUST}, status: {eq: accepted}}, inbound: enforces, not_inbound: supersedes}}, ...]}
binding: effective
rules:
  - {name: orphan_must,      severity: error, when: {any_of: [{attr: {modality: {eq: MUST}, status: {eq: accepted}}}, {attr: {modality: {eq: MUST_NOT}, status: {eq: accepted}}}], not_inbound: enforces}, message: "is MUST or MUST_NOT and accepted but nothing enforces it"}
  - {name: orphan_test,      severity: error, when: {attr: {kind: {eq: conform}}, not_outbound: enforces},                                    message: "enforces no clause"}
  - {name: stale_premise,    severity: error, when: {attr: {status: {eq: accepted}}, via: {edge: premise, attr: {status: {eq: retired}}}},   message: "is accepted but a premise is retired"}
  - {name: deviation_pressure, severity: warn, when: {attr: {status: {eq: accepted}}, inbound: {edge: deviates-from, min: 5}},               message: "has 5+ deviations; reconsider the clause"}
  - {name: no_counterexample, severity: warn, when: {attr: {kind: {eq: clause}, status: {eq: accepted}}, not_outbound: counterexample},      message: "is accepted without a counterexample"}
```

## 根拠（調査結果・出典）

### A. DocDag 自身の設計と実装（一次情報）

- README「The model」: 制約層／参照層の分離、数字連なりによる同一性、status は射影であり binding ＝
  accepted かつ未置換、辺と矛盾する status は finding。
  https://github.com/Kaikei-e/DocDag
- `docs/configuration.md`: `adr` preset 全文、`edges` の `inverse` / `max_inbound` 等、`derived_edges`、
  ルール語彙は固定かつ完全で式言語は無い、`edges:` / `rules:` はリスト置換。
  https://github.com/Kaikei-e/DocDag/blob/main/docs/configuration.md
- `internal/config/config.go`: `Config` / `EdgeSpec` / `Condition` / `Rule` の定義。preset は `Config` 値
  そのもの（「every field is expressible in docdag.yaml」）。
- `internal/graph/query.go` `Binding` / `BindingSet`: `config.EdgeSupersedes` と `isAccepted` をコードで
  固定している。D3 はこれを preset 定義の射影へ移す。
- `internal/model/model.go`: ID は preset が正規化しエンジンは比較のみ（D1 が同一性モデルを kind ごとに
  委譲できる根拠）。`Node.Attr` はスカラーのみ、`AttrList` はスカラーのリストのみ（D2 のマップ要素受理は
  `parse.Refs` と `AttrList` の拡張になる）。
- CHANGELOG v0.2.0（2026-08-27）: 「A larger constraint vocabulary, still without an expression language」。
  https://github.com/Kaikei-e/DocDag/blob/main/CHANGELOG.md

### B. 用途側の要求（uzushio 標準）が既存の実務知見と一致すること

- Anthropic「Demystifying evals for AI agents」（2026-01-09）: task / trial / grader / transcript /
  outcome / eval harness の語彙、「経路ではなく成果物を採点」、reference solution と 0% pass の扱い、
  eval-driven development。条項＝Task、適合テスト＝grader という対応の根拠。
  https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents
- Harbor task format / Reward Kit: `tests/test.sh` が `reward.json` を書く「ファイルが契約」の言語非依存
  設計、verifier の分離実行。D5 で適合テスト実体を Markdown 外に置く根拠。
  https://www.harborframework.com/docs/tasks 、https://www.harborframework.com/docs/rewardkit
- OpenAI「Harness engineering」（2026-02-11）: 不変量は機械で強制し実装は指示しない、lint のエラー文に
  修正指示を注入。R6（MUST の効力は機械検査で決まる）の実務的根拠。
  https://openai.com/index/harness-engineering/
- Böckeler「Harness engineering for coding agent users」（martinfowler.com、2026-04-02）: guides /
  sensors、computational / inferential、振る舞いハーネスが未解決、ハーネスの coverage 指標が必要。
  https://martinfowler.com/articles/harness-engineering.html
- Anthropic「Claude's constitution」（2026-01-21）: ルールが適切なのは予測可能性・評価可能性が決定的な
  ときで、それ以外は判断を育て、置くルールには理由を添える。MUST を不変量に絞る方針の根拠。
  https://www.anthropic.com/constitution
- BCP 14（RFC 2119 / RFC 8174）の SHOULD: 「特定の状況では無視する正当な理由がありうる」。
  逸脱記録（deviation）はこの「正当な理由」を文書化したもの。https://www.rfc-editor.org/rfc/rfc2119

### C. 理論的裏付け

1. **関手的データモデル（D1・D2・D4）** — Spivak: スキーマは有限表示圏、インスタンスは Set 値関手、
   スキーマ間関手 F は随伴な移行関手 Δ_F・Σ_F・Π_F を誘導する。preset 改訂を関手として扱えば移行は
   canonical に決まり、`deprecated` / `migrate_to` は Δ（列の破棄）・Σ（最小限の押し出し）に対応する。
   射影は自然変換であるべきで、実体化した導出値は移行と可換でなくなる（R1 の理論的根拠）。
   https://www.sciencedirect.com/science/article/pii/S0890540112001010 、https://arxiv.org/abs/1212.5303
2. **Institution 理論（D4）** — Goguen & Burstall: 充足関係は記法の変更のもとで不変（satisfaction
   condition）。preset 改訂が署名射に収まるなら findings は不変、変化すれば意味論の変更。R7 の diff 判定を
   minor / major の機械判定にできる根拠。https://doi.org/10.1145/147508.147524
3. **様相論理（D3）** — van Benthem の特徴付け定理: 様相論理は一階論理の双模倣不変断片。DocDag の
   `Condition` は文書グラフ上の多重様相論理そのもの（`inbound` ＝ ◇、`attr` ＝ 命題変数、`any_of` /
   `not` ＝ 結合子）。`via` は入れ子様相、`min` は段階付き様相で断片内に留まる。モデル検査は線形。
   推移閉包（`resolve`）と閉路検出は断片の外で、組み込み検査に留める理由。
   https://academic.oup.com/logcom/article/30/7/1331/5896992 （定理の現代的な解説と一般化）
4. **Defeasible deontic logic（R6・逸脱記録）** — strict rule / defeasible rule / defeater の三分法、
   constitutive / prescriptive の区別、追跡可能な構成的証明論。MUST ＝ strict、SHOULD ＝ defeasible、
   逸脱記録 ＝ defeater、射影 ＝ constitutive rule。
   Governatori, Colombo Tosatto, Rotolo (DEON 2020):
   http://collegepublications.co.uk/DEON/Governatori%20&%20Colombo%20Tosatto%20&%20Rotolo_DEON2020.pdf 、
   三分法の簡潔な整理は https://arxiv.org/abs/2203.16275 §2.2.1
5. **規範変更の論理（append-first、withdrawn / superseded）** — AGM は規範の derogation の研究から
   生まれ、derogation（廃止）と amendment（修正）は contraction / revision の類似物として特徴付けられて
   いる。`withdrawn` ＝ derogation、`superseded` ＝ amendment。
   https://plato.stanford.edu/entries/logic-belief-revision/ 、https://doi.org/10.1007/s10506-010-9097-5

### D. CUE を採用しない根拠（代替案 A）

- CUE 側の Issue #339: closed struct は実用上煩雑で、評価器のバグと定義の厳しさの両方が原因。
  https://github.com/cue-lang/cue/issues/339
- Discussion #2857 / Issue #2853: closedness アルゴリズムのメモリ消費、不完全式の再評価など性能課題。
  https://github.com/cue-lang/cue/discussions/2857
- v0.15 `explicitopen` 実験: closedness の概念自体を簡素化中。意味論が動いている。
  https://cuelang.org/docs/howto/try-explicitopen-experiment/

## 検討した代替案

- **A. CUE（または Nickel 等）で preset を記述する。** 不採用。DocDag は既に `docdag.yaml` に語彙層を
  持ち、YAML の隣に第 2 の設定言語を置くことになる。closed struct の意味論が実験中で、CI ゲートの依存と
  して不安定（根拠 §D）。必要だった機能（kind ごとの open/closed、改訂の互換判定）は D1 と D4 で足りる。
- **B. `validate` 内に Datalog エンジン（Cozo / crepe 等）を内蔵する。** 不採用。「式言語を置かない」
  設計判断と矛盾し、双模倣不変断片の外に出るためルールのメタ検査（矛盾・空虚性）の決定可能性を失う。
  再帰は `resolve` / `cycle` の組み込みで十分。探索的な合成クエリは `export --format json` で外に出す。
- **C. 辺を汎用の `rel: [{type, to, ...}]` リストで書く。** 不採用。DocDag の「キー＝辺型」モデルは
  `derived_edges` / `inverse` / `--touching` の位置情報（`KeyLines`）と結び付いており、汎用リストにすると
  finding の行番号と `fix:` が粗くなる。辺型の追加は `edges:` の 1 エントリで済むので汎用化の利得が無い。
- **D. DocDag を変えず、uzushio 側で CLI をラップして kind と属性を扱う。** 不採用。binding 射影と
  ルール評価が DocDag 内で閉じているため、ラッパは同じグラフ評価を二重実装することになる。`internal/`
  配下はライブラリとして import できず、CLI 結合では `validate --touching` やフックの利点を失う。
- **E. 導出値（binding、effective level、鮮度）を frontmatter に書く。** 不採用。DocDag の「status は
  射影」という原則と正面から矛盾し、preset 改訂で腐る（根拠 §C-1）。
- **F. kind をディレクトリだけで表し、ID は数字連なりのまま。** 不採用。条項 ID は仕様書・適合テスト・
  逸脱記録から人とエージェントが引用するため、`UZ-V-001` のような可読な接頭辞付き ID が要る。
  数字だけでは kind 間で衝突し `id_collision` の判定が意味を持たなくなる。
- **G. 複数の ADR に分割する（D1〜D5 を独立の決定として扱う）。** 今回は不採用。5 つはどれも単独では
  R1〜R7 を満たさず、`spec` preset を成立させる 1 つの決定の構成要素と判断した。非目標に挙げた 4 項目は
  独立に決められるため後続 ADR とする。

## 影響とトレードオフ

**得るもの**

- 標準の条項・適合テスト・逸脱・計測・前提を 1 つの vault で DocDag が検査でき、`orphan_must` /
  `stale_premise` / `deviation_pressure` のような「教条化の兆候」が CI の finding になる。
- Alt / Plecto の ADR 運用と同じツール・同じ語彙で標準を保守できる。`context` と `--touching` が
  そのままエージェント向けの「現行条項の地図」になる。
- binding の定義が preset に移ることで、`adr` 以外の preset が独自の効力概念を持てる。

**失うもの・引き受けるリスク**

- **同一性モデルの一般化（D1）が最大のリスク。** `IDShaped` / `documentLink` / `DefaultReferencePattern`
  は数字連なり前提で、参照層の解決も影響を受ける。`kinds:` 未指定時の挙動を bit 単位で維持する回帰
  テストが必要。
- **辺のマップ要素の受理（D2）は挙動変更。** 従来 `invalid_ref` だったマップ要素が、`attrs` を宣言した
  辺では有効になる。`attrs` 未宣言の辺では従来通りとし、CHANGELOG に明記する。
- **preset の複雑化。** `spec` preset は `adr` の数倍の設定量になる。configuration.md に kind 別の
  節を設け、`docdag new --kind` でテンプレートを kind ごとに持つ。
- **ルール語彙の拡張（`min` / `max` / `via`）は語彙の「完全性」の主張を更新する。** 追加は 2 語に限定し、
  以後の追加は「双模倣不変断片に留まるか」を審査基準として checks.md に書く。
- **性能。** 射影の評価はルールと同じ線形時間だが、射影間依存の解決（トポロジカル順）が加わる。
  1,000 文書規模で `validate` の所要時間を計測してから既定に入れる。
- **バージョニング。** `adr` preset 利用者にとっては既定不変の minor。`spec` preset は新規。JSON 出力の
  `schema_version` は `preset_version` と `projections` の追加で 2 に上げる。

**今後の課題（後続 ADR）**

- 0002 パス等式（`enforces ; resolve = enforces` のような可換性）の検査
- 0003 MAY 条項のノード化と強い許可 / SHOULD NOT の衝突検出
- 0004 `preset lint`（ルールの矛盾・空虚性、現行 vault で発火しえないルールの検出）
- 0005 有効期間（in-force 区間）からの status 射影

**実装順序の目安**

1. D3 の `binding:` 射影化（`adr` preset の出力不変を確認）
2. D2 の辺属性（`supersedes.reason` を `adr` 向けにも任意で使えるようにする）
3. D1 の `kinds:`
4. D4 の `fields:` / `preset_version`
5. `spec` preset の同梱と Plecto の既存ゲート（gate_tolerances.toml、conformance filter）を `conform` 文書
   として登録する試験運用

## 関連ADR

本 ADR が DocDag の最初の ADR。後続として次を起票済み（いずれも Proposed）。

- 0002 `target:` と `path_constraints:`（辺の合成に関する不変量）
- 0003 `modality` の 5 値化、MAY の強い許可としての明示、`modality_conflict` / `excepts` / `interop`
- 0004 `docdag lint`（ルール・射影の矛盾・空虚性・不発の 3 層検査）
- 0005 kind ごとの `period:` と as-of 射影、`--at` による transaction time

後続 ADR が本 ADR に求める修正（採択時に反映する）：

- 0003: `level` を `modality`（MUST / MUST_NOT / SHOULD / SHOULD_NOT / MAY）に改め、`kinds[].fields[]` に
  `one_of` / `required` を追加する。`effective_*` 射影は `modality` を読む。**反映済み**（本文の該当
  ブロックに「0003 により改訂」と注記）。`MUST_NOT` は `effective_must` の any_of に畳み、`binding:` は
  `effective_must` から `effective`（= effective_must ∨ effective_should ∨ 明示された MAY）に改めた。
  0003 R2 が「MAY が `--binding` に含まれる」ことを、D3 が「両方 binding のときに衝突を検出する」ことを
  要求しており、`binding: effective_must` のままでは弱い衝突が原理的に検出できないためである。
  射影名を `in_force` にしなかったのは、0005 D2 が `in_force` を `period:` から engine が計算する
  属性として定義しており、射影は同名の属性を評価時に隠すためである（0005 は `in_force: {eq: "true"}`
  を `effective` の各 alternative に足す形で乗る）。
- 0005: `deviates-from` の辺属性 `expires` を deviation ノードのフィールドに移し、`premise` の
  `status: retired` を `period: {until: retired_on}` に置き換える。`stale_premise` は `in_force` を読む。
