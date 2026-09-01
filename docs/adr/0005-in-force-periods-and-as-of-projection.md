# 0005: 有効期間を kind ごとの `period:` として宣言し、binding・現行・逸脱の効力を as-of 時点の射影にする

## ステータス

Accepted

採択日: 2026-09-01

## 日付

2026-09-02

## コンテキスト

### 「置換したが未採択」を表す語彙が無い

DocDag の現行モデルでは、文書 B が `supersedes: [A]` を宣言した瞬間に A は binding でなくなり、A の status が
`superseded` でなければ `status_drift`（error）になる。B が `proposed` でもそうなる。Season 2 の種ファイル
（2026-08-29）で指摘した `--binding` と `resolve` の非対称（resolve は proposed の葉を返し、binding は
A も B も返さない）はこの設計の帰結で、「B は提案中で、それまでは A が有効」という現実の状態を表現できない。

uzushio 標準ではこの状態が常態になる。条項の改訂は proposed → trial → accepted の段階を踏み（ADR-0001 §3）、
後継が trial の間、前身は効力を持ち続けなければならない。さらに次の事実はどれも「いつからいつまで」を持つ。

| 事実 | 出所 |
| --- | --- |
| 条項の効力開始・終了 | 標準の版のリリース日、後継条項の効力開始日 |
| 逸脱記録の期限（`expires`） | ADR-0001 D2 |
| deprecated フィールドの `sunset` | ADR-0001 D4 |
| 前提（premise）の退役日 | モデル世代の交代 |
| 計測（measure）の実施日 | ADR-0001 D5 |

現状これらは、個別のフィールドと個別のルールで扱われるか、`updated` のような鮮度メタデータ
（種ファイルで「腐る」と判定済み）に頼っている。

### 先行モデル

**法情報学。** LegalRuleML は、規則が時間とともに変化するパラメータ——status（strict / defeasible / defeater）、
validity（repealed / annulled / suspended）、jurisdiction——を持ち、効力（efficacy）や執行可能性
（enforcement）といった時間的側面を曖昧さなく表現することを設計目標に掲げる。Akoma Ntoso は
`<temporalData>` に発効（entry into force）と執行可能期間を持ち、遡及（retroactivity）や失効後効
（ultractivity）を含む法文の時間的進化を追跡する。法理論の整理では、規範の時間次元は存在（公布から）、
効力（force）、効果（efficacy）、適用可能性（applicability）に分かれ、効果が効力より前に始まる（遡及）ことも
廃止後に続く（失効後効）こともある。

Governatori と Rotolo は、法体系の abrogation（廃止、将来に向かって効力を失わせる）と annulment（無効化、
最初から無かったことにする）を扱うには、信念改訂や base revision では不十分で、**2 本の時間軸**——
法体系のある時点版の内部の時間と、法体系そのものが進化する時間——を区別する必要があるとした。

**時制データベース。** SQL:2011 は、行が「現実世界で有効だと信じられる期間」を持つ application-time period
テーブル（valid time）と、システムが行を保持していた期間を自動管理する system-versioned テーブル
（transaction time）を定義し、両方を持つ bitemporal テーブルを可能にした。期間は既存の 2 列を
`PERIOD FOR` で束ねた閉開区間で、新しいデータ型は導入せず、`AS OF SYSTEM TIME` で時点指定の問い合わせができる。

Governatori–Rotolo の 2 本の時間軸と SQL:2011 の valid / transaction time は同じ区別である。DocDag に
当てると、**valid time = frontmatter に書かれた効力期間、transaction time = git の履歴**であり、後者は
`--immutable-since` と `internal/vcs` で既に一部扱っている。

### 満たすべき要件

- R1 kind ごとに効力期間を宣言でき、既定は `date` フィールドを開始とする
- R2 binding・現行の葉・逸脱の効力・衝突検出（ADR-0003）を、指定時点（as-of）で評価できる
- R3 終了日は後継の開始日から導出でき、明示値と矛盾すれば finding
- R4 `validate` の結果が壁時計に依存しない（CI の決定性）
- R5 期間は日付粒度の閉開区間 `[from, until)`
- R6 `period:` を宣言しない kind と `adr` preset の既定挙動は変わらない
- R7 valid time（as-of）と transaction time（リビジョン）を独立に指定できる

## 決定

### D1. kind ごとに `period:` を宣言する

```yaml
kinds:
  clause:
    period: {from: in_force_from, until: in_force_until}   # 既定: from は date
  deviation:
    period: {from: date, until: expires}
  premise:
    period: {from: date, until: retired_on}
```

- 値は ISO 8601 の日付（`YYYY-MM-DD`）。時刻とタイムゾーンは持たない（R5）。
- `from` を省略した kind は `date` を開始とし、`date` も無ければ開始は −∞（常に開始済み）。
- `until` の明示値が無い文書は D3 の導出に従い、導出もできなければ +∞（開いた期間）。
- `until < from` は `period_invalid`（error、構造）。

### D2. エンジンが `in_force` 仮想属性を計算し、射影とルールはそれを読む

- as-of 時点 t について、`in_force(d) := from(d) ≤ t < until(d)` を kind に `period:` がある文書に対して
  エンジンが計算し、`attr: {in_force: {eq: "true"}}` として `rules` / `projections` / `target` から参照できる。
  日付の比較は語彙に入れず、エンジンだけが行う（ADR-0001「式言語を置かない」の境界を守る）。
- `period:` が無い kind では `in_force` は常に true。これにより R6 が成立する。
- ADR-0001 の `binding:` 射影は `spec` preset で次に改める。

```yaml
projections:
  - name: has_inforce_successor
    when: {via_inbound: {edge: supersedes, attr: {in_force: {eq: "true"}, status: {eq: accepted}}}}
  - name: effective_must
    when:
      attr: {modality: {eq: MUST}, status: {eq: accepted}, in_force: {eq: "true"}}
      inbound: enforces
      not: {attr: {has_inforce_successor: {eq: "true"}}}
binding: effective_must
```

- 「現行の葉」（ADR-0002 `leaf_of`）は、`period:` がある kind では「in_force な accepted の後継を持たない」
  に読み替える。後継が proposed / trial、または accepted だが `from` が未来なら、前身は葉のままである。

### D3. 終了日を後継から導出し、矛盾を検査する

- 明示の `until` が無い文書 A について、A を supersede する文書 S のうち `status: accepted` のものの
  `from` の最小値を `until(A)` とする（導出値。frontmatter には書かない。ADR-0001 R1）。
- 明示の `until(A)` と導出値が異なれば `period_conflict`（error、構造）。`related` に S を列挙する。
  `fix:` は出さない（`derived_conflict` と同じく、どちらが正しいかは推定しない）。
- 後継が `withdrawn` になった場合、導出は自動的に消え、A は再び開いた期間に戻る。これは
  Governatori–Rotolo の abrogation の撤回に相当し、append-first のまま表現できる。

### D4. `status_drift` を時点依存にし、`pending_successor` を加える

`spec` preset のルールを次にする。

```yaml
rules:
  - name: status_drift
    severity: error
    when:
      attr: {status: {not: superseded}}
      via_inbound: {edge: supersedes, attr: {status: {eq: accepted}, in_force: {eq: "true"}}}
    message: "an in-force successor supersedes it but status is not superseded"
  - name: pending_successor
    severity: warn
    when:
      attr: {status: {eq: accepted}}
      inbound: supersedes
      not: {attr: {has_inforce_successor: {eq: "true"}}}
    message: "a successor is declared but not yet in force; this clause remains binding until then"
  - name: premature_superseded
    severity: error
    when:
      attr: {status: {eq: superseded}}
      not: {attr: {has_inforce_successor: {eq: "true"}}}
    message: "status is superseded but no successor is in force yet"
```

- `adr` preset は変更しない（R6）。`period:` を宣言した `adr` 利用者向けに、上の 3 ルールを
  configuration.md に「時点依存の status 検査」として載せる。Alt / Plecto で導入する場合は
  `period: {from: date}` を宣言し、既存の `status_drift` を置き換える。

### D5. as-of の既定値と指定方法

| コマンド | 既定の as-of | 指定 |
| --- | --- | --- |
| `validate` | HEAD のコミッタ日付（git 外なら実行日） | `--as-of YYYY-MM-DD`、環境変数 `DOCDAG_AS_OF` |
| `query` / `resolve` / `context` / `stats` | 実行日 | 同上 |
| `lint --corpus`（ADR-0004） | `validate` と同じ | 同上 |

- `validate` を HEAD のコミッタ日付に固定するのは R4 のためである。同じコミットは何度走らせても同じ結果になる。
  「期限切れの逸脱を定期的に検出する」用途は、定期実行で `--as-of $(date -I)` を明示する。
- すべての出力（text の先頭行、JSON のヘッダ）に `as_of` を含める。`query --binding --as-of 2027-04-01` で
  「後継が効力を持ったとき何が binding か」を事前に確認できる。

### D6. transaction time：`--at <rev>`

- `--at <rev>` で、管理対象文書を指定リビジョンの内容で読む（`internal/vcs` の `File(rev, path)` を全文書に
  適用する）。`--immutable-since` が基準側で行っている読み込みの一般化である。
- `--at` と `--as-of` は独立で、組み合わせると bitemporal な問い合わせになる：
  `docdag query --binding --at v1.2.0 --as-of 2026-06-01` は「v1.2.0 時点の vault が、2026-06-01 に
  有効だと述べていた条項」を返す。
- `--at` は読み込み専用で、`new` には効かない。

### D7. 逸脱・前提・フィールド廃止への適用

- 逸脱記録（deviation）は `period: {from: date, until: expires}` を持つ。ADR-0001 D2 で `deviates-from` 辺の
  属性としていた `expires` は、**逸脱記録ノードのフィールド**に移す（辺属性を残すと期間の意味論が二重になる）。
  ADR-0003 D3 の defeater としての `excepts` と、ADR-0001 の `deviation_pressure` は、`in_force` な逸脱だけを
  数える。期限切れは `expired_deviation`（warn）。
- 前提（premise）は `until: retired_on` を持ち、ADR-0001 の `stale_premise` は `via: {edge: premise, attr: {in_force: {eq: "false"}}}` に改める。`status: retired` は不要になる（status の増殖を避ける）。
- ADR-0001 D4 の `fields[].sunset` は、フィールド定義側の日付なので本 ADR の `period:` とは別扱いのまま。
  ただし比較に使う時点は同じ as-of にする。

### 非目標

- annulment（遡及的無効化）。標準の条項が「最初から無かった」ことになる場面は稀で、必要なら
  `in_force_until: <in_force_from と同日>` と `supersede-reason: conflict` で表現できる。専用の語彙は足さない。
- 遡及（効果が効力より前に始まる）と失効後効（廃止後も効果が残る）。法情報学では必要だが、ソフトウェア標準の
  条項には過剰。効力と効果を区別しない。
- 日付より細かい粒度、タイムゾーン。
- 期間の重なり・順序を問う Allen 関係の語彙化。包含（as-of が区間内か）だけで足りる。
- 履歴全体の時系列走査（「いつ binding でなくなったか」の探索）。`--at` の 2 時点比較で足り、必要なら後続 ADR。

## 根拠（調査結果・出典）

### A. 法情報学の時間モデル

- LegalRuleML Core Specification v1.0（OASIS, 2017）「Temporal Management」：規則は時間とともに変わる
  パラメータ（status、validity、jurisdiction）を持ち、効力・執行などの時間的側面を曖昧さなく表現する。
  https://docs.oasis-open.org/legalruleml/legalruleml-core-spec/v1.0/legalruleml-core-spec-v1.0.html
- LegalRuleML の特殊化に関する報告書：規範は発効（enforceability）、効果（efficacy）、適用（applicability）の
  3 軸で区間を持ち、時間パラメータを規則の任意の部分に紐づけられる。
  https://interoperable-europe.ec.europa.eu/sites/default/files/news/2024-07/A%20LegalRuleML%20specialisation.pdf
- Akoma Ntoso の `<temporalData>`（発効・執行可能期間）と、遡及・失効後効を含む時間モデル（Palmirani）。
  https://unsceb-hlcm.github.io/part1/index-13.html 、
  https://www.balisage.net/Proceedings/vol24/html/Palmirani01/BalisageVol24-Palmirani01.html
- 法理論の時間次元（存在・効力・効果・適用可能性）と、効果が効力より前に始まる／廃止後に続く場合の整理。
  https://ceur-ws.org/Vol-1296/paper5.pdf
- Governatori, Rotolo「Changing legal systems: legal abrogations and annulments in Defeasible Logic」
  （Logic J. IGPL 18(1), 2010）：信念改訂・base revision では法の変更（特に遡及）を扱えず、法体系の時点版内部の
  時間と法体系の進化の時間という 2 本の時間軸を区別する時制 defeasible logic を提案。
  https://academic.oup.com/jigpal/article-abstract/18/1/157/655276

### B. 時制データベース

- SQL:2011 の時制機能（Kulkarni, Michels, SIGMOD Record 41(3), 2012 の解説）：application-time period
  テーブル（valid time）は「現実世界で有効だと信じられる期間」を利用者が設定し、未来の日付も入れられる。
  system-versioned テーブル（transaction time）は自動管理。 https://cs.ulb.ac.be/public/_media/teaching/infoh415/tempfeaturessql2011.pdf
- SQL:2011 概要：期間は既存 2 列を束ねた閉開区間で新データ型は無い。`AS OF SYSTEM TIME` の時点指定、
  両者を併用した bitemporal テーブル。 https://en.wikipedia.org/wiki/SQL:2011 、
  https://en.wikipedia.org/wiki/Temporal_database

### C. DocDag 一次情報

- `docs/checks.md`「Status is a projection」：`status_drift`、`superseded_orphan`、`withdrawn` の意味論。
  本 ADR の D4 はこれを時点依存に一般化する。 https://github.com/Kaikei-e/DocDag/blob/main/docs/checks.md
- `docs/ci.md` と `internal/vcs/vcs.go`：`--immutable-since` が基準リビジョンの文書を `File(rev, path)` で
  読む。D6 の `--at` はその一般化。
- `internal/model/model.go` `Node.Date`：`date` フィールドは既にノードの一級属性で、D1 の既定開始に使える。
- DocDag Season 2 種ファイル（2026-08-29、社内）：`--binding` と `resolve` の proposed に対する非対称の指摘、
  「置換したが未採択」の語彙欠如、`updated` フィールドの鮮度が腐る問題。

### D. 上位の方針

- ADR-0001 R1「導出値を書かない」：`until` の導出値を frontmatter に書かない（D3）。
- ADR-0001 §C-5：`withdrawn` ＝ derogation、`superseded` ＝ amendment。本 ADR はそこに「効力期間」を
  足し、amendment の後継が効力を持つまでの中間状態を status を増やさずに表す。

## 検討した代替案

- **A. status に `pending` / `in_amendment` / `sunset` を追加する。** 不採用。status はグラフの射影であり、
  時点依存の状態を status に焼き込むと、時点が進むたびに status を書き換える運用になる（＝鮮度メタデータと
  同じ腐り方をする）。期間を事実として持ち、状態は射影する方が一貫している。
- **B. 効力期間を git の履歴（コミット日）から推定する。** 不採用。transaction time と valid time は別で、
  「次のリリースから有効」のように未来日付の効力開始は普通にある。SQL:2011 が両者を分けた理由と同じ。
- **C. `validate` の as-of 既定を実行日にする。** 不採用。同じコミットの検証結果が日によって変わり、
  CI ゲートとして不適切。HEAD のコミッタ日付に固定し、定期実行だけ明示指定する。
- **D. 時制 defeasible logic の推論器を内蔵し、遡及・失効後効・annulment まで扱う。** 不採用。
  式言語を置かない方針に反し、ソフトウェア標準には過剰。日付比較だけをエンジンに閉じ込める。
- **E. 期間を辺の属性（`supersedes` に `effective_from`）として持つ。** 不採用。効力は文書の属性であり、
  辺に置くと 1 文書に複数の後継がある場合に意味が分裂する。辺属性は理由型（`reason`）のような
  関係固有の事実に限る。
- **F. 日時・タイムゾーンまで持つ。** 不採用。標準の効力は日単位で十分で、タイムゾーンは決定性を損なう。

## 影響とトレードオフ

**得るもの**

- 「置換したが未採択」「後継は accepted だが来月から」「期限切れの逸脱」が、status を増やさずに
  射影と finding で表現できる。`--binding` と `resolve` の非対称が解消する（proposed の後継は binding に
  影響せず、resolve は as-of で葉を返す）。
- `--as-of` で将来時点の binding 集合を事前に確認でき、標準の版のリリース計画に使える。
- `--at` と `--as-of` の組み合わせで、「あの時点の vault が何を有効だと述べていたか」を再現できる
  （インシデントの事後分析で必要になる問い）。

**失うもの・リスク**

- **エンジンが日付比較を持つ。** 語彙に式言語を入れないという境界は守るが、`in_force` はエンジン計算の
  仮想属性であり、「ルール語彙は固定かつ完全」の説明に「エンジン計算の仮想属性は `in_force` の 1 つ」を
  追記する必要がある。
- **`status_drift` の意味が preset 間で異なる。** `adr`（時点非依存）と `spec`（時点依存）で同名ルールの
  条件が違う。checks.md で両方を明記し、`period:` を宣言した `adr` 利用者には移行手順を示す。
- **ADR-0001 の `expires`（辺属性）と `status: retired`（premise）を本 ADR で改める。** ADR-0001 は
  Proposed のまま更新できるので、本 ADR の採択時に 0001 の該当箇所を修正する。
- **`--at` の実装コスト。** 全文書を `git show` 相当で読むため、1,000 文書規模では数秒かかる。
  `--immutable-since` と同じくオプション機能とし、既定経路の性能には影響させない。
- **CI の as-of が HEAD のコミッタ日付に固定されると、期限切れの検出が「次のコミット」まで遅れる。**
  定期実行（cron）で `--as-of $(date -I)` を明示する運用を ci.md に書く。
- **導出 `until` の意味論は「accepted な後継の最小 from」に固定した。** trial の後継は効力を持たないという
  ADR-0001 §3 の設計に依存する。将来 trial に効力を持たせる変更をするなら、本 ADR を supersede する。

## 実装時の逸脱（採択後に追記）

本 ADR は実装済みである。決定のうち、実装が本文と違う形を取った点を記録する。

- **D5「すべての出力に `as_of` を含める」を、JSON は常に・text は `period:` を宣言した corpus に限る、
  に改めた。** JSON のヘッダ（`validate` / `lint` / `context` / `stats`）は常に `as_of` を、`--at` が
  与えられていれば `at` を持つ。text 出力は、いずれかの kind が `period:` を宣言している場合にだけ
  末尾の要約行に `, as of <day>` を足す（error があって要約行が出ない場合は `as of <day>` の 1 行）。
  理由は 2 つある。`adr` preset は `period:` を持たず、その corpus の答えは日付に依存しないので、
  日付を印字しても読む人には意味がない。そして text の先頭行を変えると、既存の golden・composite
  action・problem matcher が一斉に壊れる — R6（既定挙動を変えない）はここにも及ぶ。
- **`period:` は kind ごとに加えて、トップレベルにも書けるようにした。** D1 は「kind ごと」だが、
  `adr` preset の corpus は kind を宣言しない（単一 kind）ので、そのままでは D4 の移行手順
  （`period: {from: date}` を宣言する）が書けない。トップレベルの `period:` は、kind が自分の
  宣言を持たないときの既定として読む — `status_values` と `fields:` が既にそうしているのと同じ規則である。
- **D7 の「`in_force` な逸脱だけを数える」を、辺インデックス全体の規則として実装した。** 効力を失った
  文書が宣言した辺は、次数の閾値・一段隣の節・`excepts` による衝突の抑止・辺の `target:` 検査から外れる
  （最後の 1 つは採択後のレビューで追補：失効した逸脱の `deviates-from` が `stale_target` として
  永久に残ると、append-first の記録がラチェットになる）。ただし
  `supersedes` 系列は除外する：終了日はそこから導出されるので、系列まで落とすと「まだ効力を持たない
  後継」という `pending_successor` が報告すべき事実そのものが消える。`path_constraints` も除外する：
  あれは「今何が有効か」ではなく corpus の形についての言明である。
- **`expired_deviation` の述語を「明示の `until` が過ぎており、かつ status がまだ `accepted`」とした。**
  D7 は「期限切れ」としか言っていないが、`until` が過ぎた文書には「後継に置き換えられて終わった条項」
  （`status_drift` / `premature_superseded` の領分）と「まだ有効だと言い張っている期限切れの記録」の
  2 種類がある。報告に値するのは後者だけである。
- **`premise` の `status: retired` は status_values に残した。** D7 は「不要になる」としているが、
  語彙から削ると既存の premise 文書が `unknown_status` になる。規則が読むのは `in_force`（＝日付）で、
  語は人が読むための prose として残す、という分担にした。

## 関連ADR

- 0001（`spec` preset、`binding:` 射影、`expires` 辺属性、`stale_premise`）— 本 ADR の採択時に D7 の通り改める（**反映済み**）
- 0002（`leaf_of`）— 「現行の葉」を as-of 時点で評価する
- 0003（`modality_conflict`、`excepts`）— 「両方が binding」と defeater の有効性を as-of 時点で評価する
- 0004（`lint --corpus`）— 層 2 の評価に `--as-of` を通す
