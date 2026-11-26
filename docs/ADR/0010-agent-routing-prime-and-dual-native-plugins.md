---
title: "docdag prime・context 抜粋フォールバック・Claude Code / Antigravity 両ネイティブプラグインでエージェント導線を確立する"
status: accepted
date: 2026-11-26
depends-on: ["0007", "0008"]
---

# 0010: `docdag prime`・`context` 抜粋フォールバック・Claude Code / Antigravity 両ネイティブプラグインでエージェント導線を確立する

## コンテキスト

DocDag は v0.2 以降 `docdag context`、`query --fields`、`query --where`、`validate --touching`、および Claude Code プラグイン（Skill と `SessionStart` / `PostToolUse` hook）を揃えてきた。しかし実際には、Claude Code や Antigravity CLI（`agy`）が管理文書を読む際に `docdag context` などのエージェント向け機能を使わず、`Read` / `Glob` / `Grep` でディレクトリを直接走査してしまう現象が起きている。

本リポジトリ自身のコーパス・同梱 fixture・プラグイン設定を実測した結果、原因は次の 4 層に分解できた。

1. **オンデマンド Skill の未発火と、常時コンテキストへのコマンド案内欠落**
   - 現行の `scripts/docdag-session-start.sh` は `docdag validate: OK: 9 docs, 8 typed edges, no cycles` の 1 行しか出力せず、`context` や `query --binding --fields id,title,status` の存在をセッション開始時に伝えていない。
   - `skills/docdag/SKILL.md` は `description` に `docs/adr/` / `docs/ADR/` / `spec/` 直接探索の代替である旨や具体的なトリガー語句が乏しく、本文も冒頭 54 行が語彙解説でコマンド表が後ろに埋もれている。また表の代表例が `docdag query --binding`（ID のみ出力）であり、タイトルが分からずエージェントが結局 `Glob` / `Read` へ戻る。
2. **Antigravity（`agy`）のネイティブプラグイン契約・Hook 契約との不整合**
   - 本リポジトリで `agy plugin validate .` を実行すると、`.antigravity-plugin/plugin.json` もルート `plugin.json` も無いため `1 error` でネイティブロード不可となる。
   - Antigravity の `hooks.json` はルート直下に置き、トップレベルをフック名で囲み、`SessionStart` と `PostInvocation` はフラット配列、`PostToolUse` は `edit_file` / `write_file` / `read_file` 等の小文字ツール名、stdin は camelCase（`toolCall.args.Path`, `cwd`, `conversationId`）、出力は `{"injectSteps": [{"systemMessage": {"systemMessage": "..."}}]}`（`PostToolUse` は `{}`）を要求する。Claude Code の `hooks/hooks.json` と `CLAUDE_PROJECT_DIR` / plain stdout / exit 2 の契約とは両立しない。
3. **`docdag context` 自体が抜粋を空で返し、パス参照も取りこぼす**
   - 本リポジトリで `./docdag context 0001` を実行すると、`brief.DefaultSection` が `"Decision"` 固定のため日本語見出し `## 決定` に一致せず、全件の抜粋が空になる。
   - `--section 決定` を明示しても、ADR 0002〜0005 のように `## 決定` の直下に `### D1. ...` サブ見出しが続く文書は、`firstParagraph` が `#` 行で即打ち切るため抜粋が空になる。
   - `preset: spec` の条文（例: `testdata/fixtures/spec-vault/spec/clauses/UZ-V-001.md`）は H1 直下に本文があり H2 を持たないため抜粋が空になり、また `spec/clauses/UZ-V-001.md` のようなファイルパス指定が `PatternNormalizer` で `unrecognized reference` になる。中身が返らないコマンドをエージェントが学習的に避けるのは自然である。
4. **CLI の自己説明性（`--help`・エラー文面）の不足**
   - `cmd/*.go` の各サブコマンドに `Example:` が無く、`query` の `Short` 説明には `--binding` への言及が無い。`unknown edge type` や `unknown document` にも有効候補や次の一手の案内が無い。

## 決定

エージェントへの導線を「常時コンテキスト」「操作時点の非ブロック案内」「オンデマンド Skill」「CLI 自身の返答品質」の 4 層で一貫させ、Claude Code と Antigravity の両方で単一リポジトリのままネイティブ動作させる。

### D1. コーパス適応型の軽量ブリーフコマンド `docdag prime` を新設する

- `docdag prime` を追加し、設定とコーパスを読んで「検証状態」「ディレクトリ／kind」「宣言された辺・フィールド・必須セクション」「このコーパスに合わせた代表コマンド（実在する ref 例つき）」を 10〜14 行程度のコンパクトなテキスト、または `--format json`（`schema_version: 1`）で出力する。
- `prime` は報告コマンドでありゲートではないため、コーパスが読み込めて出力できた場合は検証エラーがあっても exit 0 で返す（出力冒頭に `docdag validate: FAILED: ...` と `run docdag validate to see the findings` を明記する）。設定不正・空コーパスは [[0007]] に従い exit 3 で fail-closed とする。
- `--if-present` はコーパス未検出（`model.ErrNoDocuments`）のときだけ無音 exit 0 にする。壊れた `docdag.yaml` や空コーパスは引き続き exit 3 とする。
- `--hint <path>` は設定解決だけを行い（文書パースを行わない）、対象パスまたは glob が管理ディレクトリ（リポジトリルート `.` 単体や `README.md` / `template.md` 等の除外ファイルを除く）に触れるときだけ 1〜2 行のコマンド案内を返す。

### D2. `docdag context`・参照解決・`--help`・エラー文面を後方互換を保って改善する

- **`brief.Section(body, want)` の子見出し対応**: 一致した H2 の直後に sibling 見出し（`level <= matchedLevel`）が来る場合は従来どおり空とする一方、子見出し（`level > matchedLevel`、例: `## 決定` 直下の `### D1. ...`）が来る場合は子見出しテキストとその直下の最初の段落（コードフェンス前まで）を抜粋として返す。
- **既定の抜粋フォールバック（`brief.DefaultExcerpt`）**: `--section` を明示指定した場合は厳密にその見出しだけを読む。未指定時のみ、`Decision`（`Decision Outcome` 優先）→ `決定` / `結論` / `Summary` / `概要` → H2/H3 が無い文書（`spec` 条文等）は先頭本文段落 → それ以外はメタデータ見出し（`Status` / `ステータス` / `Date` / `日付` 等）を除いた最初の非空セクション、の順でフォールバックする。
- **ファイルパス参照の解決**: `normalize` で `cfg.Normalizer()` が受理しない場合、`g.Nodes` のファイルパス（相対・絶対・シンボリックリンク解決済み）と突き合わせて解決する。これにより `spec/clauses/UZ-V-001.md` のようなパスが `context` / `resolve` / `query` でそのまま通る。
- **自己説明的な `--help` とエラーヒント**: 全サブコマンドに実行可能な `Example:` を追加し、`query` の `Short` に `--binding` を明記する。`unknown edge type` には許可された辺種別を列挙し、`unknown document` / `unrecognized reference` には `docdag query --binding --fields id,title` の案内を添える。

### D3. Claude Code プラグインの Skill と Hook を強化する

- `skills/docdag/SKILL.md` を逆ピラミッド構成に改め、冒頭で管理ディレクトリへの `Glob` / `Grep` / `Read` 走査より `docdag` を優先する原則とコマンド早見表（`--fields id,title,status` 付き）を提示する。フロントマターは Agent Skills 標準の `name` とトリガー語句を充実させた `description` のみに留め、両エージェントで警告無く読める形を保つ。
- `scripts/docdag-session-start.sh` はコーパス存在判定後に `docdag prime` を呼び出し（`prime` を持たない旧バイナリでは従来の `docdag validate` サマリへフォールバック）、セッション開始時に検証状態（空の既知ディレクトリ時の fail-closed 通知を含む）とコーパス適応コマンド指針を同時に注入する。
- `hooks/hooks.json` に非ブロック型の `PreToolUse`（matcher: `Read|Glob|Grep`）を追加し、`scripts/docdag-pre-read.sh` が `docdag prime --if-present --hint <target>` の結果を `hookSpecificOutput.additionalContext`（`permissionDecision` は付けず既存の権限規則を尊重）として注入する。管理外パスでは即座に無音 exit 0 とする。

### D4. 単一リポジトリで Antigravity（`agy`）ネイティブプラグインを併設する

- Claude Code 用の `.claude-plugin/`・`hooks/hooks.json` と衝突しないパスに、Antigravity ネイティブ構成を併設する:
  - `.antigravity-plugin/plugin.json`
  - ルート `hooks.json`（Antigravity スキーマ）
  - `rules/AGENTS.md`（プラグイン有効時に常時マージされる軽量ルール）
  - `scripts/agy-session-start.sh`（コーパス存在判定後に `docdag prime` を `injectSteps` 経由で注入）
  - `scripts/agy-post-tool.sh` + `scripts/agy-post-invocation.sh`（Antigravity の `PostToolUse` は `{}` のみ返す契約のため、編集後の `validate --touching --if-managed` / `lint --config` 結果および初回読み取り時の `--hint` を会話単位のキューへ記録し、直後の `PostInvocation` で `injectSteps` としてモデルへ届ける）
- 共有 Asset である `skills/docdag/SKILL.md` は 1 箇所で維持し、CI とテストで `.antigravity-plugin/plugin.json`・ルート `hooks.json`・`agy plugin validate --strict .` 準拠を検証する。

## 根拠（調査結果・出典）

- **実測（本リポジトリ）** — `./docdag context 0001` は全 6 文書で抜粋 0 件、`--section 決定` でも 0002〜0005 が空、`./docdag query --binding` は ID のみ 9 行、`agy plugin validate .` は `plugin.json` 不在で 1 error だった。
- **実測（`testdata/fixtures/spec-vault`）** — `UZ-V-001.md` や `UZ-V-006.md` は H1 直下に条文本文があり H2 を持たないため `docdag context` の抜粋が空になり、`spec/clauses/UZ-V-001.md` のパス指定は `PatternNormalizer` で拒否された。
- **Claude Code Hooks / Skills 仕様** — `SessionStart` の標準出力および `PreToolUse` の `hookSpecificOutput.additionalContext` でモデルへ文脈を注入でき、`PreToolUse` で `permissionDecision` を省略すれば実行を止めず既存の権限判定を維持できる。<https://code.claude.com/docs/en/hooks> / <https://code.claude.com/docs/en/skills>
- **Antigravity カスタマイズ仕様** — `.antigravity-plugin/plugin.json` を持つネイティブプラグインはルート `hooks.json`・`rules/AGENTS.md`・`skills/*/SKILL.md` を直接読み込み、Hook は camelCase JSON 入力と `injectSteps`（`SessionStart` / `PostInvocation`）・`{}`（`PostToolUse`）の出力契約に従う。<https://antigravity.google/docs/plugins> / <https://antigravity.google/docs/hooks> / <https://antigravity.google/docs/rules>
- **Agent Skills 仕様** — `SKILL.md` のオンデマンド段階開示は `name` と `description` による自動選択に依存するため、常時有効なルーティング層（`SessionStart` / `rules/AGENTS.md`）と役割を分ける必要がある。<https://agentskills.io/>

## 検討した代替案

- **MCP サーバーを新設する** — ローカル単一バイナリの CLI に対して常駐プロセスと大量の JSON スキーマ定義を常時コンテキストへ載せるコストが釣り合わず、パイプ合成や CI との共通性も失われる。CLI のまま `prime` とプラグイン導線を整える方を採った。
- **`PreToolUse` で `Glob` や `Read` をブロック（`deny`）する** — エージェントが `context` の抜粋を読んだあとに本文全体を確認したい正当な `Read` まで妨げ、回避ループを招く。非ブロックの `additionalContext`（Antigravity は会話単位 1 回限りの `PostInvocation` 通知）に留めた。
- **新コマンドを増やさずシェルスクリプトだけで案内を組み立てる** — `docdag.yaml` のカスタム `dir:`・`preset: spec` の 8 種類・宣言済み `fields:`・`sections.required` を POSIX `sh` で正確に解釈するのは脆弱で、[[0007]] の教訓（フックと本体のディレクトリ判定の乖離）を再発させる。設定解釈は Go 本体（`docdag prime`）に一元化した。
- **`docdag.yaml` に新しい `context_section:` キーを足す** — [[0006]] の公開スキーマ変更となり、既存コーパスは設定を書き換えるまで改善の恩恵を受けられない。既定フォールバックで設定追加なしに効かせる方を採った。
- **Claude Code プラグインのみを維持し、`agy plugin import` の変換に任せる** — 変換ではツール名（`Edit|Write` vs `edit_file|write_file`）、ペイロードのキー名、`SessionStart` / `PostToolUse` の出力 JSON 契約の違いが吸収されず、さらに Antigravity 固有の `rules/AGENTS.md` も提供できない。

## 影響とトレードオフ

- **得るもの** — Claude Code と Antigravity の両方で、セッション開始時にコーパス固有の `docdag` 導線が常時入り、管理文書への直接読み取り時にも `docdag context` への気付きが入り、`context` 自体も日本語 ADR・H3 分割 ADR・`spec` 条文で抜粋を返すようになる。
- **コマンド集合の拡張** — 公開サブコマンドが 9 個から 10 個（`prime` 追加）になる。既存の終了コード規約（0/1/2/3）と他コマンドの出力契約は維持する。
- **`context` の既定出力の変化** — `spec` コーパスや非英語 ADR コーパスでこれまで空だった `excerpt` が埋まるようになる。`--section` を明示した呼び出しや `Decision` 見出しを持つ既存 MADR コーパスの出力は変わらない。
- **Antigravity の `PostToolUse` → `PostInvocation` 状態ファイル** — 編集検証や読み取りヒントを `PostInvocation` へ受け渡すため `${PLUGIN_DATA}`（未設定時は一時ディレクトリ）に会話 ID ごとの小さなファイルを作る。会話 ID は `[A-Za-z0-9_-]` に正規化し、読み出し時に原子的リネームと削除を行う。
- **二重プラグイン定義の保守費** — `.claude-plugin/plugin.json` と `.antigravity-plugin/plugin.json` のバージョンおよび 2 系統の Hook スクリプトを同期して保つ必要がある。両系統のテストを `cmd/plugin_test.go` と CI に載せて乖離を防ぐ。

## 関連ADR

- [[0006]] — 公開 `config` 契約を維持するため、`docdag.yaml` に必須の新キーを増やさず CLI 側のフォールバックで解決した。
- [[0007]] — fail-closed の原則。`docdag prime` も空コーパスや不正設定を exit 3 で落とし、`--if-present` でも設定不正は隠さない。
- [[0008]] — `sections:` で宣言された見出し構造を `docdag prime` の出力に反映し、執筆前に必須セクションをエージェントへ伝える。
