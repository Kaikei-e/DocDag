# **DocDag（v0.5.0）技術戦略の再検証とアーキテクチャ進化ロードマップ：設計制約・形式検証基盤・プロトコル統合の精査**

ソフトウェア工学において、AIコーディングエージェントの推論と実装を拘束する「決定論的検証器（Deterministic Verifier）」の重要性が急速に高まっている1。Go言語で実装されたドキュメントグラフ検証エンジンであるDocDag（github.com/Kaikei-e/DocDag）は、YAMLフロントマターを持つMarkdownコーパスから型付き有向非巡回グラフ（DAG）を抽出し、グラフ不変条件の強制とクエリ解決を単一バイナリおよび公開Go APIとして提供する基盤である1。  
先行調査レポート（以下「旧レポート」）では、DocDagを取り巻く外部エコシステム（仕様駆動開発、MCP、LSP、要件管理ツール群）の動向整理が行われた一方で、DocDag自身のv0.5.0（2026年9月27日リリース）時点における実装到達点の把握に重大な事実誤認が含まれていた1。その結果、既に実装済みの機能（ダイアグラム出力、Reviewdog形式出力、文書作成時の自動ステータス更新、着信リンク制約、置換系譜の葉への到達検証など）が新規開発項目として再提案されただけでなく、DocDagが意図的に課している設計制約（追記専用モデル、決定可能性を維持するためのパス長制限、外部依存ゼロの原則）と正面から衝突する提案がなされていた1。さらに、文書品質の観点からも、利用者データの裏付けを欠いた誇張表現、古いREADMEへの循環参照、無関係なリポジトリ（CMoA）の誤引用と構文崩れ、および図表と本文の矛盾（AST走査と字句スキャンの混同）といった問題が散見された1。  
本レポートでは、DocDag本体の一次情報（README.md、CHANGELOG、docs/commands.md、docs/checks.md、docs/configuration.md、および公開Goパッケージ群）と外部ツールの一次ソースを突き合わせ、旧レポートの事実誤認と設計上の矛盾を全面的に訂正する1。その上で、DocDagの真の技術的優位性である「何を形式的に検査できるか（二時点モデル、様相衝突と例外抑制、空虚性検査、Fail-Closed設計）」を軸に据え、単独メンテナンスの保守コスト、非目標（Non-Goals）、成功指標、および主要利用先（AltのADR/PM vaultおよびuzushioの規範vault）の実需に即した実践的進化ロードマップを体系化する1。

## **1\. 旧レポートにおける事実誤認の検証と一次ソースに基づく訂正**

旧レポートにおける最大の構造的欠陥は、リポジトリのmainブランチに残存していた古いREADME.md（同梱プリセットをadrのみとし、インストール例をv0.2.0に固定していた記述）に過度に依拠し、v0.3.0からv0.5.0にかけて実装されたdocs/配下のリファレンスおよびCHANGELOGの精査を怠った点にある1。人間および調査AIの双方が最初に参照するエントリポイントの乖離が誤認の連鎖と循環引用（主張の大半が古いREADME自身に依拠する構造）を生んでおり、この事実自体が「README.mdをv0.5.0の実態に合わせて更新すること」が最も費用対効果の高い最優先施策であることを実証している1。

### **1.1 DocDag（v0.5.0）の実装到達点に関する訂正**

旧レポートが「未実装の拡張ターゲット」として扱った機能の多くは、v0.5.0時点で既にコア機能として稼働している1。第一に、グラフの視覚化・ダイアグラム出力は大半が実装済みである1。旧レポートではinternal/renderを拡張してMermaidやGraphviz DOT出力を追加することをPhase 1の主要施策として提案していたが、docs/commands.mdに定義されている通り、docdag exportサブコマンドは標準の--formatフラグとは独立してmermaid\|dot\|jsonを受け取り、既定でMermaid構文を標準出力へ書き出す実装を既に備えている1。したがって、視覚化に関して実際に残されている拡張余地は、ゼロからのエクスポータ実装ではなく、現在有効（binding）なノードと被置換（superseded）ノードのスタイル・色分けの自動付与、docdag queryやdocdag contextの抽出結果をそのままサブグラフとして描画する導線の接続、およびD2フォーマット対応といった小規模な差分拡張に限られる1。  
第二に、CIおよび静的解析向けの出力形式の把握が不完全であった1。旧レポートは出力形式としてtext、github、およびタブ区切りの--fieldsのみを挙げ、Pull Requestコメント連携のためにSARIF出力の追加を優先課題としていた1。しかし実際には、docdag validateおよびdocdag lintはgithubに加えてrdjson（Reviewdog Diagnostic JSON）形式での出力を既にサポートしており、reviewdogを経由してGitHubのPR上にインラインコメントを投稿する経路は確立されている1。SARIFはGitHub Code Scanningダッシュボードへ統合する場合の5番目の出力形式という位置付けに過ぎず、その限界効用は小さい1。  
第三に、新規文書作成時における被置換文書のステータス更新は既に自動化されている1。旧レポートは「新規ADRが0002をsupersedesしたにもかかわらず、0002側のstatusがacceptedのまま残るステータスドリフト」を例に挙げてvalidate \--fixの必要性を論じた1。しかし、docdag new（内部パッケージinternal/newdoc）は、新規ノード生成時に被置換文書のstatus値のみをピンポイントで書き換え、本文および改行コードをバイト単位で完全保持した上で、ディスクへの書き込み前にすべての変更をメモリ上で事前計算するトランザクション的な安全性を既に実現している1。したがって、validate \--fixが真に必要となるのは、人間やAIエージェントがdocdag newを介さずにテキストエディタ等で直接フロントマターのsupersedesを手書き編集した場合に限定される1。  
第四に、着信側（インバウンド）カバレッジ検証と上流変更の波及（Suspect）検知も、既存のルール語彙で大部分が記述可能である1。v0.3.0時点で、DocDagのルール定義語彙にはインバウンド・アウトバウンドの次数しきい値（inbound／outbound）および1ホップ先のノード条件を検査するvia／via\_inboundが実装されている1。Sphinx-Needs（v8.2.0）のnetwork\_backに相当する着信側からの検証ルールはユーザーがdocdag.yamlで自由に宣言でき、specプリセットに組み込まれているorphan\_must（適合性テストを持たないMUST条項の検出）はその一適用例に過ぎない1。また、上流文書の改訂に伴う古さの検知についても、target:制約のleaf\_ofによって置換系譜（supersession lineage）の現在の葉（最新ノード）を指すことを要求し、古い中間ノードを参照しているエッジをstale\_targetとして報告した上で、修正候補（Fix）に現在の葉のIDを提示する機構が既に稼働している1。追記優先（append-first）の文書管理モデルにおいては、このleaf\_ofとstale\_targetの組み合わせこそがグラフネイティブなSuspect検知として機能する1。

### **1.2 外部ツール・プロトコル動向および文書品質に関する一次情報の精密化**

外部エコシステムに関する記述は概ね事実と合致していたものの、一次ソースに照らして複数の細部を精密に訂正する必要がある1。まず、公式MCP Go SDK（github.com/modelcontextprotocol/go-sdk）については、v1.7.0以降（2026年9月時点でv1.8.0）において2026-07-28版のMCP仕様に完全対応しており、同仕様（SEP-2577）でroots・sampling・loggingが正式に非推奨（Deprecated）となった点、およびSEP-2575／SEP-2567によりinitializeハンドシェイクを廃したステートレス・セッションレスモデルへ移行した点は事実である1。ただし、旧レポートが「GoogleおよびAnthropicの共同メンテナンス」と記述した点は不正確であり、リポジトリ公式の定義は「Googleとの協力でメンテナンスされている公式SDK（Maintained in collaboration with Google）」である1。また、旧レポートのアーキテクチャ図にあった「stdio / SSE」というトランスポート表記は、2026-07-28仕様ではレガシーなHTTP+SSEが廃止されstdioとStreamable HTTP（および単一長寿命リクエストのsubscriptions/listen）の2系統に統一されているため古い記述である1。  
次に、OpenSpecにおけるspec-driven-with-adrの位置付けについて、旧レポートはこれをOpenSpec本体の標準スキーマであるかのように記述したが、正しくはintent-driven-dev組織のopenspec-schemasリポジトリで公開されているカスタムスキーマである1。また、AnthropicによるLSP推奨の文脈についても、旧レポートは日本語の二次ブログ記事を経由して「AnthropicがAIエージェント展開の事前準備としてLSP導入を推奨している」と論じたが、一次ソース（Claude Code公式ドキュメントのCode Intelligenceプラグイン仕様等）において、LSPはあくまで型付きプログラミング言語におけるシンボル単位のナビゲーション（定義ジャンプ、参照検索、ホバーでの型情報取得）およびコンパイラ要らずの自動型エラー検出として位置付けられている1。これをMarkdownのドキュメントvaultに適用するのは「正規化IDをプログラム上のシンボルに見立てる」という設計上の類推（アナロジー）であり、Anthropic自身がMarkdown vaultへのLSP適用を推奨しているわけではない点は割り引いて評価すべきである1。  
さらに、競合ツールであるcontextlintの実装状況に関して、旧レポートの比較表ではMCP対応を「未対応」とし、コンテキスト圧縮機能を「一部対応」と過小評価していたが、contextlintはv0.7までにMCP統合を完了しており、変更の影響集合やコンテキストスライスを抽出するAPI（contextlint impact / slice）も既に公開している1。一方、2026年4月のGenerative Specification白書が主張する「確率的なLLM自身を検証器にせず、決定論的ツールに検証させる」という原則は、DocDagが元々取っている立場と完全に一致している1。なお、旧レポートでモノレポ連邦（Federation）の説明に引用されていた文献（Kaikei-e/CMoA）は、docdag@v0.3.0を内部で利用している別プロジェクト（Common Mixture-of-Agents）のリポジトリであり、Federation自体の根拠としては無関係であった上、構文の崩れや図と本文の用語不一致（AST走査と字句スキャンの混在）が含まれていたため、本レポートではこれらを完全に排除・統一する1。  
以下の表は、旧レポートにおける主要な記述と、一次情報に基づく検証・訂正結果を対比したものである1。

| 検証対象領域 | 旧レポートの記述・提案内容 | 一次ソースに基づく事実関係（v0.5.0時点） | 戦略的インプリケーション |
| :---- | :---- | :---- | :---- |
| **ダイアグラム出力** | renderパッケージにMermaid / DOT出力を新規追加（Phase 1） | docdag exportがmermaid\|dot\|jsonを既にサポート（既定はmermaid標準出力） | ゼロからの実装は不要。binding/supersededの色分けとquery/context連携のみに絞る |
| **診断出力形式** | text / github / \--fieldsのみと認識し、SARIF追加を重視 | validateとlintは既にrdjson出力に対応し、reviewdog経由のPRコメント経路が存在 | SARIFは5番目の形式となり限界効用が低い。優先度を引き下げる |
| **置換時の状態更新** | 新規ADRが旧ADRを置換した際のステータス不整合を--fixの主用途に例示 | docdag newが被置換文書のstatus値のみをバイト保持で自動更新済み | validate \--fixの対象は、人間やエージェントが手動でsupersedesを編集した場合に限定 |
| **着信検証・古さ検知** | Sphinx-Needsのnetwork\_backや上流変更検知を新規導入すべきと主張 | v0.3.0でinbound/outbound次数・via\_inboundを実装済み。上流の古さもleaf\_ofとstale\_targetで検知可能 | 既存ルール語彙で大部分が表現可能。新規プリミティブ追加の前に既存語彙の活用を徹底 |
| **MCPトランスポートと競合** | 「stdio / SSE」と表記。DocDagのようなMCPサーバーは市場に未存在と主張 | 2026-07-28仕様はstdioとStreamable HTTPのみ。contextlintがv0.7でMCP統合済み | MCP自体は差別化要因にならない。ローカルvault用途ならstdioの薄いToolsのみで十分 |
| **外部引用と用語整合性** | OpenSpec本体スキーマと誤認、二次ブログ経由のLSP推奨、CMoA誤引用、AST/字句スキャンの矛盾 | spec-driven-with-adrはopenspec-schemasのカスタム定義。LSP適用は型付き言語からの類推 | 誇張表現と無関係な引用を排し、設計制約と実運用コストに基づく客観的評価へ移行 |

## **2\. DocDagの設計思想と数学的・構造的制約の精査**

旧レポートが掲げた「既存のコア設計を一切破壊することなく4つのピラーを追加できる」という主張は、DocDagが意図的に維持している形式的な設計制約を見落とした誇張である1。CHANGELOGおよびADR群が明示している通り、DocDagの堅牢性と決定論的な高速性は、「何を機能として足すか」ではなく「何をシステムから排除するか」という厳格な制約によって成立している1。

### **2.1 コアエンジンが死守している4つの形式的制約**

DocDagのコアアーキテクチャは、4つの厳格な形式的制約の上に構築されている1。第一の制約は「式言語（Expression Language）の排除」であり、ルール記述にチューリング完全な式言語や汎用スクリプトを持ち込まず、宣言的なグラフ語彙に限定することで、設定の解析可能性とメタ検証（docdag lint）の完全性を担保している1。第二の制約は「様相（Modality）の深さ2固定」であり、MUSTやSHOULDなどの規範的様相の伝播・衝突検査（modality\_conflict）において探索深度を2に固定することで、計算量の爆発と意味論の曖昧化を防いでいる1。  
第三の、そして理論上最も重要な制約が「パス制約（path\_constraints）の1〜2ステップ制限」である1。グラフ上のパス制約を1〜2ステップまでに制限しているのは単なる実装上の省略ではなく、3ステップ以上の任意長パス制約を許容すると、ルール間の含意問題（Implication Problem：あるルールが別のルールに論理的に包含されるか、空虚にならないかを判定する問題）が数学的に決定不能（Undecidable）に陥るためである1。docdag lintによる重複・矛盾ルール検出が成立するのは、このステップ数制限があればこそである1。第四の制約は「外部依存ゼロの原則（Zero New Dependencies）」であり、リリースごとに新規の外部ライブラリ依存を追加しない方針が貫かれ、YAMLパーサ（github.com/goccy/go-yaml）を除けばGo標準ライブラリのみで単一バイナリが完結している1。

### **2.2 旧レポートの提案が引き起こす設計思想との衝突と解消策**

上記の設計原則に照らすと、旧レポートの提案には4つの深刻なアーキテクチャ上の衝突が存在する1。  
第一の衝突は、Doorstop式SHA256フィンガープリントとappend\_only／--immutable-sinceの不変性モデルとの構造的矛盾である1。Doorstopにおける変更波及（Suspect）検知では、リンクは親アイテムのUIDと「最後にレビューした時点の親のSHA256フィンガープリント（URLセーフBase64）」の組として**子アイテム側のファイル内**に保存される1。すなわち、親文書の改訂を確認してレビュー完了とする行為は、子文書のフロントマターを書き換える操作を意味する1。しかしDocDagのspecプリセットでは、適合性テスト（conform）や測定（measure）といった下位Kindはappend\_only（追記専用）として定義されている1。さらに、--immutable-since \<rev\>フラグの制約下では、確定済み文書に対して許容される変更は「status値の変更」「inverseキーへの追記」「本文末尾への行追加」の3点のみに厳格に限定されている1。したがって、Doorstop方式を模倣してconform文書のフロントマターにレビュー済みハッシュ値を書き込めば、即座にimmutable\_violationエラーを引き起こすことになる1。  
この矛盾を解消し、DocDagの設計哲学と整合させる代替案は2つ存在する1。1つ目の代替案は「依存される規範文書をその場で書き換えさせない（不変規範＋置換系譜モデル）」であり、規範内容の変更は必ず新規ノードによるsupersedes（置換）でのみ表現させ、下位文書が古い条項を参照し続けている状態は既存のtarget:制約におけるleaf\_of（stale\_target違反）に任せる方式である1。2つ目の代替案は、その場での書き換えが運用上必要なKindに対して「レビューを『状態の更新』ではなく『追記されるイベント』として記録する」方式である1。具体的には、レビュー記録を独立した追記型の記録（例えば、対象ノードIDとレビュー時点のGitリビジョンを属性に持つエッジ）として表現し、対象文書の古さ（Staleness）はinternal/vcsが既に読み込んでいるGit履歴から決定論的に導出する1。DocDagはトランザクション時間を既に--at \<rev\>で扱っているため、この方式は二時点モデルとも完全に整合する1。  
第二の衝突は、コード・テスト境界スキャンおよび連邦（Federation）による同一性モデルの破壊である1。ソースコード内のコメント注釈の走査、テスト実行レポートの取り込み、Spec Kit／Kiroアダプタ、およびマルチディレクトリ連邦（Federation）は、いずれもDocDagの入力形式または同一性モデル（Identity Model）を根本から拡張するものであり、「既存コアを一切破壊しない」という旧レポートの主張と矛盾する1。特に連邦（Federation）は、「ディレクトリがkindを決定し、kindが同一性規則（ID正規化規則）を決定する」という現在のDocDagの根幹モデルそのものに手を入れる必要があり、旧レポート自身が実装難易度を「高」と分類していたこととも自己矛盾している1。  
第三の衝突は、SDDプリセットにおいて一時的な成果物までを検証対象に含める提案が、引用元記事の主張自体と逆向きになっている点である1。旧レポートが引用したintent-driven.devの記事自体が、変更提案（proposal）、設計（design）、タスク（tasks）は変更の出荷後にアーカイブされる一時的な足場（Scaffolding）に過ぎず、将来の変更が依拠する恒久的な成果物（Load-bearing artifacts）はspecsとADRの2つだけであると明確に述べている1。DocDagが検証すべきは恒久的な成果物の方であり、ディレクトリ構成の変化が速いSpec KitやKiroに対しては、保守コストの高い組み込みアダプタを実装するのではなく、docs/にdocdag.yamlの設定レシピを掲載する程度が妥当である1。  
第四の衝突は、validate \--fixにおける「確定的な機械的修正」と「推測」の混在である1。現在、公開パッケージmodel.FindingのFixフィールドは自由形式の文字列（Fix string）であり、そこには"set status: superseded in ..."のような機械的な修正と、"did you mean 0001?"のような推測（候補提示）が同居している1。無条件に自動適用できるのは前者のみであるため、--fixを実装するには、公開model.Findingに互換性を保った追加変更として「構造化された編集フィールド」を別途新設し、確定的な修正と推測を型レベルで分離することが先決となる1。また、DocDagには「グラフから導出された終了日はフロントマターに書き戻さない」という原則があるため、--fixがグラフ射影や導出値をファイルへ実体化（Materialize）しないよう明示的に除外する必要がある1。

## **3\. MCP／LSP統合戦略の現実的再評価と公開エンジンAPI設計**

AIエージェントおよびエディタとの統合プロトコルであるMCPとLSPについても、一面的な礼賛を排し、トークン効率、実装コスト、およびGoモジュール境界の観点から再評価を行う必要がある1。

### **3.1 MCP（Model Context Protocol）の費用対効果と最小スコープ設計**

旧レポートはMCPサーバー化を既存のCLI＋スキル連携の「上位互換」として扱っていた1。しかし、Anthropic自身の分析が示している通り、エージェントに接続するMCPツールが増えるほど、セッション開始時のツール定義の事前ロードと、呼び出しごとの中間結果のコンテキスト通過がエージェントの応答を遅くし、トークンコストを押し上げる要因となる1。DocDagの現在の形（高速なCLI、Claude Codeスキル、および--budget付きのdocdag contextによる要約抽出）は、既に極めて軽量かつトークン効率に優れた設計である1。また、contextlintがv0.7までにMCP統合を完了していることからも明らかな通り、MCP対応そのものはもはや市場における差別化要因にはならない1。  
したがって、DocDagにおけるMCPの価値は、CLIの代替ではなく「シェル実行環境を持たないエージェントクライアントへの到達」に限定される1。実装する場合は、既存のJSON出力や要約出力をそのまま返す薄いTools（context、validate \--touching、query／resolve）のみに絞り込み、Resourcesプリミティブの実装は実需が見えてからで十分である1。また、トランスポートについても、2026-07-28仕様では旧来のHTTP+SSEがStreamable HTTP（ステートレス対応）に置き換えられているが、ローカルvault用途であればstdioトランスポートのみで完全に事足りる1。

### **3.2 LSP（Language Server Protocol）における4つの技術的コストと依存ゼロ実装**

MCPとは対照的に、LSPサーバーの提供価値は本物である1。具体的には、ファイル保存時の自動診断（publishDiagnostics）、正規化IDやWikilinkの定義ジャンプ（definition）、およびinternal/briefを流用したカーソルホバー表示（hover）が人間の執筆体験とエージェントの探索精度の双方を向上させる1。ただし、実装にあたっては旧レポートが触れていなかった4つの技術的コストを精査する必要がある1。  
第一のコストは「未保存バッファの扱い」であり、現在のinternal/parseはワーキングツリーかGitリビジョン（--at）から直接ファイルを読む設計であるため、未保存のインメモリ編集を扱うにはオーバーレイ（仮想ファイルシステム：VFS）層が必要となる1。第二のコストは「グラフ検査には全コーパスが必要になること」であり、Sphinx-NeedsもドキュメントでIDE文脈における即時返却可能なローカル検査と全体インデックス構築を要するネットワーク検査を明確に区別している1。もっとも、数百件規模のMarkdown vaultであれば、複雑なインクリメンタル計算エンジンを作らずとも、ファイル保存（didSave）のたびに全コーパスを再構築すればミリ秒単位で完了するため、初期実装ではdidSaveトリガーに絞ることで第一・第二のコストを同時に最小化できる1。  
第三のコストは「位置情報の変換」であり、DocDagのmodel.Locationは1始まりの行・列（1-based Line/Column）であるのに対し、LSP仕様は0始まりでUTF-16コード単位のRange（0-based UTF-16 offset）を要求するため、マルチバイト文字を考慮した変換層が不可欠となる1。第四のコストは「他のMarkdown LSPとの共存」であり、ワークスペース内の.mdファイル全体に無差別に反応して他のLSPと競合しないよう、docdag.yamlの存在によって起動範囲を厳密に絞る必要がある1。  
実装ライブラリに関して、旧レポートが挙げたtliron/glspは自らのREADMEで「初期リリースであり一部機能は未実装」と明記している1。DocDagの薄いLSPが必要とするメソッドは4〜5個に過ぎないため、外部ライブラリに頼らずGo標準ライブラリのみでJSON-RPC 2.0を手書きし、「外部依存ゼロ」を保つアプローチが極めて現実的である1。また配布面でも、DocDagは既にClaude Codeプラグインであり、.claude-plugin/plugin.jsonでlspServersを宣言して\${CLAUDE\_PLUGIN\_ROOT}でバンドルしたバイナリを指し示せるため、既存資産を活かして安価に配布できる1。

### **3.3 アダプタ配置と公開エンジンAPIに関するアーキテクチャ決定（ADRの先行策定）**

MCPもLSPも、内部パッケージであるinternal/graphとinternal/briefの機能を必要とする1。Goのモジュール仕様上、別モジュールからはinternal/をインポートできないため、実装に着手する前にアダプタの置き場所をDocDag自身のADRとして決定しておく必要がある1。選択肢は「本体モジュールに依存を足して内部パッケージを直接呼ぶ」か、「アダプタ用の薄い公開エンジンAPIを切り出す」かの2つに集約される1。後者のアプローチは、v0.4.0（ADR 0006）でconfigやmodel、lintを公開パッケージ化した流れの自然な延長線上にあり、uzushioやCMoAのようにGoプログラムからDocDagをインプロセスで呼び出す利用側にも大きな恩恵をもたらす1。

## **4\. エコシステム比較の再構築：「どこで動くか」から「何を検査できるか」へ**

旧レポートの比較表は、DocDagのREADMEの機能一覧から比較軸を抽出していたため、構造的にDocDagが勝つ表になっていた上、DocDagが弱い軸（採用実績、静的サイト公開、エディタ体験）が欠落していた1。さらに、競合であるcontextlintは既にLSP・MCP・CIの三層で同一ルールを走らせる形態を完成させている1。したがって、DocDagが打ち出すべき真の独自性は「どこで動くか」ではなく「何を検査できるか」にあり、具体的には旧レポートでほとんど扱われていなかった4つの形式的検証能力に集約される1。

### **4.1 DocDagの本質的優位性を成す4つのコア検証能力**

第一の核心的能力は、--as-of \<date\>と--at \<rev\>を組み合わせた二時点モデル（Bitemporal Model）である1。--as-ofは文書フロントマターの有効期間（period:）と置換関係を評価して指定日時点の有効性（Valid Time）を射影し、--atはinternal/vcsを通じて指定Gitリビジョン時点のコーパス状態（Transaction Time）を復元する1。これにより、任意の過去時点における規範と決定の有効状態を数学的に再現できる1。  
第二の核心的能力は、exceptsによる抑制付きの様相衝突検査（modality\_conflict）である1。規範文書間のエッジに沿った様相（MUST、SHOULD等）の矛盾を検出するだけでなく、明示的な例外エッジ（excepts）による衝突の抑制と、不要な例外宣言を弾くexcepts\_strict、および期限切れの例外を検出するexpired\_deviationを備えることで、厳格な標準規格と期限付き特例（Deviation）の運用を単一グラフ上で両立させている1。  
第三の核心的能力は、フィクスチャ付きのdocdag lintによる空虚性検査（Vacuity Check）である1。設定されたルール群を実コーパスおよび違反フィクスチャ群と突き合わせ、一度も発火しない空虚なルール（Dead Rules）、常に発火するルール、冗長な重複ルール、およびフィクスチャとの不一致を検出することで、検証ルール自体の腐敗を防止している1。  
第四の核心的能力は、**v0.5.0におけるFail-Closed（安全側失敗）化**である1。未知のフィールドや未定義のエッジ属性、管理外ファイルの混入（unmanaged\_file）等を暗黙に無視せず検証エラーとして厳格に弾くFail-Closed設計により、AIエージェントのハルシネーションやドリフトに対する決定論的な防波堤として機能する1。

### **4.2 偏りを排した客観的エコシステム比較表**

これら4つの検証能力と、DocDagが現時点で弱い軸（採用実績、静的サイト生成、エディタ体験）を公平に盛り込んだ比較表を以下に示す1。

| 評価軸・技術特性 | DocDag (v0.5.0) | contextlint (v0.9+) | Sphinx-Needs (v8.2+) | Doorstop | Log4brains |
| :---- | :---- | :---- | :---- | :---- | :---- |
| **ランタイム・外部依存** | Go単一バイナリ / Go API（外部依存はgo-yamlのみ） | TypeScript / Node.js | Python \+ Sphinx拡張 | Python CLI / パッケージ | TypeScript / Node.js |
| **二層DAGとパス・次数制約** | 制約層と参照層を分離。via\_inbound、1〜2ステップパス制約、逆エッジ検証 | 3ゾーン間のリンク検証、必須セクション・テーブル構造検証 | networkおよび着信側network\_backスキーマ検証 | 単一ルートからのツリー階層検証 | 置換（Supersedes）関係のタイムライン抽出のみ |
| **二時点モデル（Valid / Transaction Time）** | **\--as-of \<date\>（有効期間射影）と--at \<rev\>（Git履歴時点）の二時点対応** | 非対応（draft/review/stableの安定度カラム検証） | 非対応（ビルド時点の静的スナップショットとVariant分岐） | 非対応（現在ワークツリーの静的状態のみ） | Gitログからの作成・更新日推測のみ |
| **規範様相の衝突と例外抑制** | **modality\_conflict、exceptsによる抑制、excepts\_strict、expired\_deviation** | 非対応 | 非対応（フィルタ文字列・条件式によるカスタム検証） | 非対応 | 非対応 |
| **ルール自体の空虚性（Vacuity）検査** | **フィクスチャ突合によるdocdag lint（空虚・全発火・重複検出）** | 設定スキーマ検証のみ（フィクスチャ空虚性検査なし） | JSON Schemaによる設定構文検証のみ | なし | なし |
| **Fail-Closed性と不変性保証** | **v0.5.0のFail-Closed化、--immutable-since \<rev\>、leaf\_ofによるstale\_target検知** | contextlint impactによる変更影響集合の算出 | ビルド時の動的関数・外部JSON依存追跡 | 子アイテムへの親SHA256ハッシュ保存（追記専用と衝突） | なし |
| **エージェント用コンテキスト圧縮** | docdag context \--budget（トークン上限内の解決先・近傍・抜粋合成） | **contextlint slice / impactによる影響集合・コンテキストスライスAPI** | AIアシスタント向けcontext7.json出力（v8.2.0〜） | なし | なし |
| **CI・ダイアグラム出力** | text、github、rdjson、--fields、export (mermaid\|dot\|json) | CLI出力、CI連携 | HTML、PDF、ReqIF、PlantUML / Graphviz描画 | CLI、HTML / Markdown / LaTeX出力 | タイムラインUI出力 |
| **エディタ体験（LSP）・MCP統合** | Claude Code Plugin（Hook / Skill）対応。**LSP・MCPは未実装（弱点）** | **LSP \+ VS Code拡張（v0.9〜）、MCPサーバー（v0.7〜）完備** | VS Code拡張（ubCode）による補完・ナビゲーション | CLI中心（専用LSP・MCPなし） | ローカルWebホットリロードプレビュー |
| **静的サイト公開（SSG）・採用実績** | **SSG機能なし。主にAlt / uzushio等の特化vaultで利用（外部実績は発展途上）** | SDD実践プロジェクト向けにOSS展開・実績拡大中 | **車載・機能安全（ISO 26262等）で多数の導入実績・SSG標準装備** | **宇宙・ROS（Space ROS等）・組込み要件管理で実績多数** | **ADR閲覧用静的Webサイト生成（SSG）が主機能・実績多数** |

## **5\. 非目標・リスク・保守コストの定義と対案ロードマップ**

単独メンテナンスのコストを抑えつつ、現在の主な利用先である「AltのADR/PM vault」と「uzushioの規範vault」における実効性を最大化するため、非目標（Non-Goals）の明確化と7段階の優先順位付けを行う1。

### **5.1 修正版・7段階の実装優先順位と各段階の設計・リスク・成功指標**

AltのADR/PM vaultおよびuzushioの規範vaultを軸に据えた場合、以下の順序で施策を進めるのが最も妥当である1。さらに、これらの採否判断自体をDocDagリポジトリのADR（0010以降）として起票し、提案を採用するにせよ退けるにせよ、その理由がsupersedesや関連エッジの系譜として残る形を取るのが最もDocDagらしい進め方となる1。  
第一段階（優先度1）は「README.mdの更新」である1。mainブランチのREADMEに残っている「adrプリセット1つのみ同梱」や「v0.2.0固定のインストール例」といった古い記述をv0.5.0の実態に合わせ、二時点モデルや空虚性検査、既存のexport・rdjson出力を正確に反映する1。今回の調査事案自体が証明している通り、人間と調査AIの双方が最初に読むREADMEの更新は最も安価で即効性の高い施策である1。  
第二段階（優先度2）は「構造化Fixと、確定的な修正だけを適用するvalidate \--fixの実装」である1。公開パッケージmodel.Findingに機械的編集を表す構造化フィールドを追加し、"did you mean 0001?"のような推測と確定的な修正を分離した上で、導出された終了日（Derived End Date）や射影ステータスをフロントマターへ書き戻さないよう明示的に除外する1。この基盤はCLIのvalidate \--fixと後続のLSPのcodeActionの共通基盤として機能する1。  
第三段階（優先度3）は「docdag exportの差分拡張」である1。既にmermaid\|dot\|jsonをサポートしているexportコマンドに対し、bindingとsupersededでノードの色やスタイルを変える機能と、docdag query／docdag contextの結果をそのままサブグラフとして描く導線（および必要に応じたD2出力）を追加する1。  
第四段階（優先度4）は「保存時診断・定義ジャンプ・ホバーに絞った薄いLSPの実装」である1。事前にアダプタの置き場所（本体モジュールへの追加か、薄い公開エンジンAPIの切り出しか）をADRで決定した上で、未実装機能の残るtliron/glspに頼らず標準ライブラリのみで4〜5個のJSON-RPCメソッドを手書きして依存ゼロを保つ1。実装に際しては、docdag.yamlの存在による起動範囲の制限、1始まりから0始まりUTF-16単位へのRange変換、およびdidSaveごとの全コーパス再構築によって4つの技術的コストを解消し、既存Claude Codeプラグインのplugin.json（lspServersおよび\${CLAUDE\_PLUGIN\_ROOT}）を通じて配布する1。  
第五段階（優先度5）は「『レビューを追記イベントとして記録する』設計のADR化」である1。specプリセットのappend\_only（conform・measure）および--immutable-sinceと構造的に衝突するDoorstop式のハッシュ上書き方式に代わり、その場で書き換えが必要なKindについてはレビュー記録を対象IDとGitリビジョンを属性に持つ独立した追記型の辺（または記録）として扱い、古さをinternal/vcsのGit履歴と二時点モデルから導出する設計をADRとして固める1。  
第六段階（優先度6）は「薄いMCPサーバーの実装」である1。シェルを持たないクライアントからの具体的な需要が見えてから、既存のJSON出力をそのまま返す薄いTools（context、validate \--touching、query／resolve）に絞り、stdioトランスポートのみで薄く実装する1。  
第七段階（優先度7）は「保留項目の非目標ADR化」である1。ソースコード注釈の走査、テストレポートの取り込み、複数vaultの連邦（Federation）、およびSpec Kit／Kiroアダプタは、入力形式や同一性モデルを肥大化させ単独メンテナンスの保守コストを押し上げるため、uzushioかAltで具体的に必要になるまで保留とし、その保留理由を非目標（Non-Goal）のADRとして記録する1。  
以下の表は、これら7段階の施策について、対象パッケージ、技術的リスクと保守コスト、ADR記録方針、および定量的な成功指標を整理したものである1。

| 優先順位・施策項目 | 対象ファイル・パッケージ | リスク・単独保守コスト | 対応ADR（0010以降） | 成功指標（Success Metrics） |
| :---- | :---- | :---- | :---- | :---- |
| **1\. READMEの更新とレシピ整備** | README.md, docs/ | **極小**: コード変更なし。ドキュメント間の同期のみ | 既存仕様の反映（必要に応じてSDD併用レシピをdocs/に追加） | 人間・AIによるv0.5.0機能（spec、export、rdjson、4大検証機能）の誤認ゼロ |
| **2\. 構造化Fixとvalidate \--fix** | model, internal/graph, cmd/docdag | **低**: 公開modelへのフィールド追加は後方互換を維持可能 | 確定的修正と推測の分離、および導出値の書き戻し禁止に関するADR | 手動編集によるstatus\_driftや逆エッジ不整合がバイト保持で100%自動修復され、導出値の実体化がゼロ |
| **3\. exportの小規模拡張** | internal/render, cmd/docdag | **低**: 既存mermaid\|dot\|json出力器への属性付与とパイプ接続のみ | 既存export仕様の小幅拡張として実装 | query / contextの結果からbindingとsupersededが色分けされたサブグラフを1コマンドで生成可能 |
| **4\. 薄型・依存ゼロLSPの実装** | cmd/docdag（または公開エンジンAPI経由） | **中**: UTF-16変換とdocdag.yaml起動ガードが必要だが、標準ライブラリ手書きで外部依存リスクはゼロ | **公開エンジンAPI切り出しの是非**および**薄型LSPスコープ定義のADR** | 外部依存追加ゼロ、数百件規模vaultでのdidSave再構築50ms以内、plugin.jsonのlspServers経由で即時稼働 |
| **5\. 追記型レビュー設計のADR化** | docs/ADR/（将来的にinternal/vcs, internal/graph） | **低（設計段階）**: Doorstop式ハッシュによるimmutable\_violationリスクを事前排除 | **レビューを追記イベントとGit履歴導出で扱う設計のADR** | append\_onlyおよび--immutable-sinceと完全整合する変更波及検知仕様の確立 |
| **6\. 薄型MCP（需要確認後）** | 別モジュールまたはcmd/docdag | **低〜中**: 公式Go SDKを使う場合は公開エンジンAPI経由で別モジュール化すると本体依存を汚染しない | シェル非搭載クライアント向け最小MCP構成のADR | 4つの薄いTools（stdio）のみで既存CLIと同等のJSON/要約レスポンスを返却 |
| **7\. コード境界・連邦等の保留** | docs/ADR/（コード変更なし） | **ゼロ（凍結により高コストを回避）**: 同一性モデル破壊と決定不能化リスクを遮断 | **非目標（Non-Goals）および保留条件を明記したADR** | 再提案時にdocdag contextで保留理由のADRが即座に引ける状態の維持 |

## **6\. 結論：今後の戦略的比重（内製Vault深化 vs 外部OSS拡大）に応じた意思決定**

本再調査で検証した通り、DocDag（v0.5.0）は二層DAGモデル、二時点（--as-of／--at）の有効性射影、exceptsによる抑制付きのmodality\_conflict検査、フィクスチャを用いたdocdag lintによる空虚性検査、およびFail-Closed設計によって、他のリンターが到達していない形式的検証領域を既に確立している1。  
最後に、フィードバックの末尾で提起された「今後DocDagを『自分のvault群（Alt / uzushio）のための道具』として磨く比重と、『外部ユーザーを獲得するOSS』として広げる比重の、どちらを重く見るか」という問いに対する構造的な分析を提示する1。この比重の置き方によって、第5章で定義した7段階の優先順位は明確に2つの異なる軌道へと分岐する1。  
第一の軌道である「自分のvault群（AltのADR/PM vaultおよびuzushioの規範vault）のための道具として磨く比重」を高く置く場合、開発の最優先事項は日々の仕様策定・設計記録の執筆と保守の摩擦を極小化することに置かれる1。この軌道では、既にClaude CodeのCLI・スキル・Hook連携が軽量に機能しているためMCPサーバー化は後回しとなり、「Step 1（README更新）→ Step 2（構造化Fixとvalidate \--fix）→ Step 4（保存時診断・定義ジャンプ・ホバーに絞った薄いLSP）→ Step 5（追記イベント型レビュー設計のADR化）」が最前線に繰り上がる1。また、LSPは標準ライブラリで手書きして本体モジュールに収めつつ、uzushioのようにGoから直接使う側の利便性を高めるために「薄い公開エンジンAPI」を切り出す判断が極めて高い投資対効果を生む1。  
第二の軌道である「外部ユーザーを獲得するOSSとして広げる比重」を高く置く場合、開発の最優先事項は初見ユーザーの認知障壁の打破と多様なエージェント環境へのリーチ拡大へとシフトする1。この軌道では、「Step 1（READMEの全面刷新と4つのコア検証強みの明文化）→ 客観的比較表およびSDD併用レシピ（docs/）の整備 → 薄い公開エンジンAPIの切り出しとStep 6（シェルを持たないクライアント向けの薄いMCPサーバー）→ Step 3（PR上での視認性を高めるexportのスタイル付け拡張）」が前面に出ることになる1。  
どちらの比重を選択するにせよ、すべての起点となるのは**Step 1（README.mdのv0.5.0化）とアダプタ用の薄い公開エンジンAPIおよび非目標に関するADR（0010以降）の起票**である1。単独メンテナンスの持続可能性と形式的整合性を損なわない観点からは、まず基底として「Altおよびuzushioのための道具としての深化（Fix・LSP・公開エンジンAPI・追記型レビューADR）」を優先してコア体験を研ぎ澄まし、その成果とドキュメント（README・比較表）の整備をもって外部OSSとしての自然な認知拡大へと繋げる段階的アプローチが最も堅実であると結論付けられる1。

#### **引用文献**

> 1. DocDagの技術的進化の調査.pdf  
> 2. DocDag reads a directory of Markdown documents with ... \- GitHub, [https\://github.com/Kaikei-e/DocDag](https://github.com/Kaikei-e/DocDag)  
> 3. go-sdk/docs/protocol.md at main \- GitHub, [https\://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)  
> 4. MCP新仕様がGo SDKで通らない、Stateless=trueに加え必須が3つ, [https\://qiita.com/kai\_kou/items/ee0efebf5f4840ce2c25](https://qiita.com/kai_kou/items/ee0efebf5f4840ce2c25)  
> 5. Releases · modelcontextprotocol/go-sdk \- GitHub, [https\://github.com/modelcontextprotocol/go-sdk/releases](https://github.com/modelcontextprotocol/go-sdk/releases)  
> 6. mcp package \- github.com/modelcontextprotocol/go-sdk/mcp, [https\://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)  
> 7. go-sdk/mcp/streamable.go at main \- GitHub, [https\://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/streamable.go](https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/streamable.go)  
> 8. MCP Go SDK 入門 \- Zenn, [https\://zenn.dev/kaonavi/articles/c0db2e8c0b2612](https://zenn.dev/kaonavi/articles/c0db2e8c0b2612)  
> 9. Claude CodeにLSPツールが追加 \- laiso, [https\://blog.lai.so/claude-code-lsp/](https://blog.lai.so/claude-code-lsp/)  
> 10. Claude CodeのLSPを公式マーケットプレイスなしで利用する, [https\://dev.classmethod.jp/articles/claude-code-lsp-from-local-marketplace/](https://dev.classmethod.jp/articles/claude-code-lsp-from-local-marketplace/)  
> 11. Markdown ドキュメント間の整合性を静的解析する contextlint, [https\://koborin.ai/ja/tech/contextlint-introduction/](https://koborin.ai/ja/tech/contextlint-introduction/)  
> 12. GitHub \- Kaikei-e/CMoA: Common Mixture-of-Agents. A general, [https\://github.com/Kaikei-e/CMoA](https://github.com/Kaikei-e/CMoA)  
> 13. Beta SDKs for the 2026-07-28 MCP Spec Release Candidate Are, [https\://blog.modelcontextprotocol.io/posts/sdk-betas-2026-07-28/](https://blog.modelcontextprotocol.io/posts/sdk-betas-2026-07-28/)  
> 14. config package \- github.com/Kaikei-e/DocDag/internal/config \- Go, [https\://pkg.go.dev/github.com/Kaikei-e/DocDag/internal/config](https://pkg.go.dev/github.com/Kaikei-e/DocDag/internal/config)  
> 15. model package \- github.com/Kaikei-e/DocDag/model \- Go Packages, [https\://pkg.go.dev/github.com/Kaikei-e/DocDag@v0.4.1/model](https://pkg.go.dev/github.com/Kaikei-e/DocDag@v0.4.1/model)  
> 16. tliron/glsp: Language Server Protocol SDK for Go \- GitHub, [https\://github.com/tliron/glsp](https://github.com/tliron/glsp)  
> 17. claude-howto/07-plugins/README.md at main \- GitHub, [https\://github.com/luongnv89/claude-howto/blob/main/07-plugins/README.md](https://github.com/luongnv89/claude-howto/blob/main/07-plugins/README.md)  
> 18. Claude Codeで自作プラグインを開発する方法をまとめた \- Qiita, [https\://qiita.com/miruky/items/753395f62397c5ca6cbd](https://qiita.com/miruky/items/753395f62397c5ca6cbd)  
> 19. lint package \- github.com/Kaikei-e/DocDag/lint \- Go Packages, [https\://pkg.go.dev/github.com/Kaikei-e/DocDag/lint](https://pkg.go.dev/github.com/Kaikei-e/DocDag/lint)  
> 20. v1.8.0でサーバーを作る手順とmcp-goとの違い \- 株式会社一創, [https\://www\.issoh.co.jp/tech/details/8272/](https://www.issoh.co.jp/tech/details/8272/)