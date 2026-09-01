# 0002: 辺の合成に関する不変量を `target:` と `path_constraints:` として検査する

## ステータス

Accepted

採択日: 2026-09-01

## 日付

2026-09-02

## コンテキスト

### 1 ホップのルールでは表現できない不変量がある

ADR-0001 で採用した `spec` preset の条項グラフには、2 本の辺の合成に関わる不変量が少なくとも 4 つある。

| 不変量 | 破れたときに起きること |
| --- | --- |
| I1. `enforces` の対象は `supersedes` 系譜の現行の葉でなければならない | 適合テストが退役した条項を「強制」し続け、後継条項は `orphan_must` になる。テストは緑のまま標準の効力が消える |
| I2. `deviates-from` の対象は binding な条項でなければならない | 退役した条項からの「逸脱」が記録され、逸脱圧（`deviation_pressure`）の統計が汚染される |
| I3. `measures` の対象は現行の葉でなければならない | 旧版条項の一致率が最新の計測として `context` に出る |
| I4. （`adr` preset）`depends-on` の対象が superseded であってはならない | 退役した契約を現行として引く。DocDag Season 2 の種ファイル（2026-08-29）で「グラフが繋いでいない矛盾」の代表例として挙げた失敗そのもの |

いずれも「辺 X の先にある文書が、辺 Y について葉である／binding である」という形で、現行の `Condition`
（`inbound` / `not_inbound` / `outbound` / `not_outbound` / `attr` / `any_of` / `not`）では書けない。
ADR-0001 が足す `via` は隣接文書の**属性**しか見ないので、隣接文書の**辺**を見る I1〜I4 は依然として範囲外である。

### DocDag の現状

- ルールは各ノードで局所的に評価される。`Resolve(g, id, t)` は辺型 t を辿って葉を返す推移的な操作だが、
  ルール語彙からは呼べない。
- `cycle` / `cardinality` / `inverse_mismatch` は設定で有効化される構造検査で、severity は固定・引き下げ不可。
- 「現行」の定義は README と checks.md で「accepted かつ何にも supersede されていない」と明文化されている。

### 理論的な制約

- Buneman・Fan・Weinstein（PODS 1998）は、半構造データ上のパス制約の含意問題が一般には r.e. 完全
  （有限含意は co-r.e. 完全）であり、断片に限れば決定可能で、その断片で逆関係や局所的な制約を表現できる
  ことを示した。後続研究では、決定的データモデル上でワイルドカードなしの Pc の含意は 3 乗時間で決定可能、
  ワイルドカード付き Pcw も決定可能、正規表現で経路を表す Pc* は決定不能である。
- ただし DocDag が行うのは制約集合の**含意**の判定ではなく、有限グラフに対する**モデル検査**であり、
  これは常に決定可能である。問題は決定可能性ではなく表現力の膨張で、ADR-0001 §C-3 が採用した
  「ルール語彙は様相論理の双模倣不変断片に留める」という設計判断を、パス制約を足しても維持できるかにある。
- Spivak の関手的データモデルでは、スキーマは有向多重グラフと**パス等式**の組で、パス等式は
  可換図式（2 つの経路が同じ写像を与える）として表現される。I1〜I4 はその退化形（片方の経路が恒等）である。

### 満たすべき要件

- R1 「辺 X の対象は辺 Y について葉である」を宣言できる（I1・I3・I4）
- R2 「辺 X の対象は条件 C を満たす」を宣言できる（I2：binding = accepted ∧ not_inbound supersedes）
- R3 finding は辺を宣言した frontmatter キーの行に位置し、`fix:` に正しい葉を提示する
- R4 評価は文書数とルール数に対して線形に近い
- R5 正規表現・ワイルドカード・任意長の経路は導入しない
- R6 `adr` preset の既定挙動を変えない（Alt の 985 ADR には superseded な文書へ `depends-on` を張った
  記録が既にあり、既定で error にすると CI が止まる）

## 決定

### D1. 辺仕様に `target:` を追加し、対象文書に対する条件を宣言する

```yaml
edges:
  - name: enforces
    key: enforces
    from: [conform]
    to: [clause]
    target:
      leaf_of: supersedes                # 糖衣: not_inbound: supersedes
  - name: deviates-from
    key: deviates-from
    from: [deviation]
    to: [clause]
    target:
      attr: {status: {eq: accepted}}
      not_inbound: supersedes            # = binding
```

- `target:` の値は `rules[].when` と同じ `Condition` で、辺の**対象**文書に対して評価される。
- `leaf_of: <edge>` は `not_inbound: <edge>` の糖衣。読み手に意図（「現行の葉」）が伝わる名前を残す。
- `target:` の中に `via` / `target` を入れ子にすることは認めない。これにより様相の深さは
  「辺 X → 対象の局所条件」の 2 で固定され、ADR-0001 の断片内に留まる。
- `target:` は `derived_edges` 由来の辺にも適用する（`status: superseded by 0003` から導いた辺も同じ対象条件を負う）。

### D2. 対象条件の違反は構造検査 `stale_target`（error）として報告する

```
spec/conform/v-001.md:5: ERROR stale_target conform/v-001: enforces targets UZ-V-001, which UZ-V-001a supersedes
  fix: did you mean UZ-V-001a?
```

- finding は辺キーの行に置く（`KeyLines` を使う）。`related` に対象と、`leaf_of` の場合はその後継を列挙する。
- `fix:` は `leaf_of` のときだけ出す。現行の `Resolve` で系譜の葉を求め、葉が 1 つならその ID を提示し、
  複数（分岐）なら列挙する。**検査は局所（深さ 2）、修正提案だけが推移的**という分担で、推移閉包を
  ルール語彙に入れない。
- severity は `cardinality` と同じく固定 error。`structural:` で引き下げられない。設定しなければ発火しない。

### D3. 長さ 2 の可換制約を `path_constraints:` として追加する（第 2 段階）

```yaml
path_constraints:
  - name: amend_targets_current
    path: [amends, supersedes]       # d --amends--> x --supersedes--> y
    equals: none                     # そのような y が存在してはならない（x は葉）
  - name: deviation_scope_consistent
    path: [deviates-from, premise]
    subset_of: [premise]             # 逸脱先条項の前提は、逸脱記録自身の前提に含まれる
```

- 各ノード d について、`path` を順に辿って到達する集合 P(d) と、`equals` / `subset_of` に書いた経路で
  到達する集合 Q(d) を比較する。`none` は空集合。
- `path` と比較経路は長さ 2 以下、要素は `edges:` に宣言された辺名のみ。逆向き辿りは辺名に `^` を前置する
  （`^supersedes` = 入辺）。ワイルドカード・正規表現・繰り返しは受け付けず、設定エラー（exit 3）にする。
- 違反は `path_mismatch`（error、固定）。finding は d の先頭辺キーの行に置き、`related` に P(d)−Q(d) の
  差分を列挙する。`fix:` は出さない（どちらの経路が誤りか DocDag は推定しない。`derived_conflict` と同じ方針）。
- D1 で表現できる制約を `path_constraints:` で書き直すことは認めない（`lint` で `prefer_target` の warn）。
  D3 は D1 で書けない場合に限る。

### D4. preset への適用

- `spec` preset：`enforces` / `measures` に `leaf_of: supersedes`、`deviates-from` に binding 条件を既定で入れる。
- `adr` preset：**既定では入れない**（R6）。configuration.md に次の 3 行を「推奨する追加設定」として載せる。

```yaml
edges:
  - {name: depends-on, key: depends-on, acyclic: true, direction: forward, target: {leaf_of: supersedes}}
```

  Alt / Plecto では、まず `stale_target` を warn 相当の運用（CI では `--format json` で件数だけ監視）で
  導入し、既存の違反を解消してから設定に入れる。

### D5. 実装順序と非目標

1. `target:` と `stale_target`（D1・D2）。`leaf_of` の `fix:` に `Resolve` を接続する。
2. `spec` preset への既定組み込み（D4）。
3. `path_constraints:` と `path_mismatch`（D3）。D1 の運用で「書けない不変量」が実際に 1 件以上出てから着手する。

非目標：正規パス制約、ワイルドカード、長さ 3 以上の経路、関数従属（キー制約）、含意問題の判定、
辺の対象に対する `via` の入れ子。

## 根拠（調査結果・出典）

### A. DocDag 一次情報

- README「The model」：binding ＝ accepted かつ未置換。`resolve` は supersedes を辿って現行の葉を返す。
  https://github.com/Kaikei-e/DocDag
- `docs/checks.md`：`cardinality` / `inverse_mismatch` / `derived_conflict` は設定で有効化される固定 severity の
  構造検査で、`derived_conflict` は「どちらが誤りか DocDag は推定しない」。D2・D3 の方針の前例。
  https://github.com/Kaikei-e/DocDag/blob/main/docs/checks.md
- `internal/graph/query.go` `Resolve` / `Ancestors` / `Descendants`：推移的な辿りは既に組み込みで存在し、
  ルール語彙からは独立している。D2 の「修正提案だけ推移的」はこの既存 API を再利用する。
- `internal/config/config.go` `EdgeSpec`：`MaxInbound` 等の宣言的な辺制約が既にあり、`target:` は同じ場所に
  収まる。

### B. パス制約の理論

- Buneman, Fan, Weinstein「Path Constraints on Semistructured and Structured Data」（PODS 1998）：
  半構造データではパス制約の含意問題が r.e. 完全、有限含意が co-r.e. 完全だが、いくつかの断片では決定可能で、
  逆関係や局所的なデータベース制約を表現するにはその断片で足りる。
  https://www.research.ed.ac.uk/en/publications/path-constraints-on-semistructured-and-structured-data/
- Buneman, Fan, Weinstein「Query Optimization for Semistructured Data using Path Constraints in a Deterministic
  Data Model」：決定的モデル上で Pc の含意は 3 乗時間で決定可能かつ有限公理化可能、Pcw も決定可能、
  正規表現を使う Pc* は決定不能。R5（正規表現を入れない）の根拠。
  https://www.research.ed.ac.uk/en/publications/query-optimization-for-semistructured-data-using-path-constraints/
- Alechina, Demri, de Rijke「A modal perspective on path constraints」（J. Logic and Computation, 2004）：
  パス制約を様相論理の側から扱う研究。D1 が「対象条件は深さ 2 の様相式」と位置づける根拠。
  （書誌は https://link.springer.com/chapter/10.1007/978-3-540-28629-5_68 の参考文献 [Alechina et al.] を参照）
- Spivak「Functorial data migration」：スキーマは有限表示圏（有向多重グラフ＋パス等式）。
  https://www.sciencedirect.com/science/article/pii/S0890540112001010

### C. 動機となった実例

- DocDag Season 2 種ファイル（2026-08-29、社内）：「退役した契約を現行として引く」を、グラフが繋いでいない
  矛盾の代表例として整理。I4 の出所。
- ADR-0001 §影響：`orphan_must` は 1 ホップの検査であり、テストが旧版条項を指し続けるケースを検出できない。
  本 ADR の I1 はその穴を塞ぐ。

## 検討した代替案

- **A. `via` を拡張し、隣接文書に対する完全な `Condition` を許す（`via: {edge: enforces, when: {...}}`）。**
  不採用（ただし D1 は実質これを「辺仕様側」に置いたもの）。ルール側に置くと、同じ制約を `enforces` を
  持つ全 kind のルールに繰り返し書くことになり、また `via` の入れ子を許すと様相の深さが無制限になる。
  辺仕様側に 1 段だけ置く方が、宣言場所が一意で深さも固定される。
- **B. 正規パス制約（`path: [supersedes+]` のような繰り返し）を導入する。** 不採用。含意が決定不能な断片に
  入る上、DocDag は既に `Resolve` で推移閉包を組み込みとして持っており、ユーザー定義の繰り返しは要らない。
- **C. `resolve` の結果を仮想属性として公開し、`attr` で比較する（`attr: {resolved: {eq: self}}`）。**
  不採用。属性比較に「自分自身」や「他ノードの ID」を持ち込むと、`attr` が局所的な命題変数でなくなり、
  双模倣不変性が壊れる。
- **D. uzushio 側のラッパで検査する。** 不採用（ADR-0001 代替案 D と同じ理由）。`Resolve` と `KeyLines` を
  持つのは DocDag だけで、二重実装になる。
- **E. `adr` preset の `depends-on` に既定で `leaf_of` を入れる。** 不採用（R6）。既存 vault の CI を止める。
  推奨設定として文書化し、運用で段階導入する。

## 影響とトレードオフ

**得るもの**

- I1〜I4 が CI の finding になり、「テストは緑だが標準は死んでいる」状態を検出できる。
- `fix:` が正しい葉を提示するため、エージェントが `--touching` の結果からそのまま修正できる。
- 語彙の追加は `target:`（既存 `Condition` の再利用）と `leaf_of`（糖衣）だけで、様相の深さは 2 に固定。

**失うもの・リスク**

- **語彙の「完全性」の主張を再度更新する。** ADR-0001 で 2 語（`min`/`max`、`via`）を足したのに続き、
  本 ADR で `target:` を足す。checks.md に「辺仕様に置ける条件は対象の局所条件のみ、入れ子不可」と
  明記し、以後の追加審査基準（双模倣不変断片に留まるか）を再掲する。
- **`derived_edges` 由来の辺に `target:` を適用すると、MADR 互換の `status: superseded by X` が
  `stale_target` を出す場面が増える。** `adr` preset では既定で `target:` を入れないので影響なし。
  `spec` preset は `derived_edges` を使わない。
- **分岐した系譜（葉が複数）では `fix:` が一意にならない。** 列挙にとどめ、選択は人に委ねる。
- **`path_constraints:` は表現力の割に使用頻度が低い可能性がある。** D5 の通り、実需が出るまで実装しない。
- 性能：`target:` は辺ごとに対象の局所条件を 1 回評価するだけで、辺数に線形。`path_constraints:` は
  長さ 2 の経路の集合比較で、ノードあたり出次数の積に比例する。1,000 文書規模では問題にならない見込みだが、
  実装時に `validate` の所要時間を計測する。

## 実装時の注記（採択後に追記）

- **D5 の段階ゲート（3 の `path_constraints:` は「書けない不変量が実際に 1 件以上出てから」）は、
  意識的に通さなかった。** 0001〜0005 は一続きの系列として一度に実装したため、D1 の運用実績を待つ
  期間そのものが存在しない。`path_constraints:` は 0.3.0 で D3 の通り出荷してあり、ゲートは
  「実需の確認を省いた」という判断の記録として残す。使用頻度が想定より低ければ、`lint` の
  `never_fired` がそれを報告する — 実需の検証は運用後に回した、というのが正味の変更である。

## 関連ADR

- 0001（`spec` preset・`via`・`projections`）— 本 ADR は 0001 の `Condition` と `kinds` を前提にする
- 0004（`preset lint`）— `prefer_target` の warn と、`target:` / `path_constraints:` の空虚性検査を 0004 に委ねる
- 0005（有効期間）— 「現行の葉」の定義が時点依存になった場合、`leaf_of` は 0005 の as-of 時点で評価する
