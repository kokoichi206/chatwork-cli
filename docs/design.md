# chatwork-cli 設計書（レビュー用ドラフト）

既存の Slack 向け CLI と同等の体験を Chatwork で実現する。
AI agent が人間の代理として Chatwork を読み書きするための CLI。

## 1. 前提となる外部仕様（Chatwork API v2）

一次ソース: https://developer.chatwork.com/docs/endpoints.md / https://developer.chatwork.com/llms.txt

- ベース URL: `https://api.chatwork.com/v2`(HTTPS 必須)
- 認証: HTTP ヘッダー `x-chatworktoken: <APIトークン>`
  - API トークンは **ユーザー本人の権限そのもの** で動く。bot アカウントの概念がなく、
    トークンで投稿すれば本人としての投稿になる。→ 要件「本人の代わりとして投稿」は API トークンだけで満たせる
  - OAuth 2.0 も存在するが、client 登録が必要で個人利用の CLI には過剰。v1 では採用しない
- レート制限:
  - 全体: 300 回 / 5 分
  - メッセージ投稿・タスク追加: 10 回 / 10 秒
  - 超過時 429。`x-ratelimit-limit / remaining / reset` ヘッダーで残量が取れる
- レスポンス: JSON。エラーは `errors` 配列
- メッセージ記法(投稿時にクライアント側で組み立てる):
  - メンション: `[To:{account_id}]`
  - 返信: `[rp aid={account_id} to={room_id}-{message_id}]`
  - 引用: `[qt][qtmeta aid={account_id} time={unix_time}]...[/qt]`
  - その他: `[info]`, `[hr]`, `[picon]` など
- 主要エンドポイント: `/me`, `/my/status`, `/my/tasks`, `/rooms`,
  `/rooms/{id}/messages`(取得は最大 100 件・`force=1` で再取得), `/rooms/{id}/members`,
  `/rooms/{id}/tasks`, `/rooms/{id}/files`

## 2. 技術選定

| 案 | 内容 | 長所 | 短所 |
|---|---|---|---|
| A. Go + cobra(推奨) | シングルバイナリ、goreleaser で配布 | 配布が最も簡単(brew tap / go install / バイナリ直置き)、標準ライブラリだけで HTTP・JSON が完結、テーブル駆動テストの文化 | 参考にした既存 CLI(TS) とコードは共有できない |
| B. TypeScript + Bun | 参考にした既存 CLI と同じ構成 | 既存 CLI の構造を流用しやすい | 実行に Bun が要る(compile しても配布サイズ大)、ランタイム管理が増える |
| C. Deno compile | TS でシングルバイナリ | TS + 単一バイナリの両取り | エコシステムが相対的に小さい |

**推奨: A(Go)**。理由は「配布が簡単」を最優先要件と解釈したため。
AI agent からの使いやすさは言語ではなく CLI の入出力設計(§6)で決まる。

依存は最小限に:
- `spf13/cobra`(サブコマンド)
- それ以外は標準ライブラリ(`net/http`, `encoding/json`)で API クライアントを自作
  (Chatwork API は小さいので SDK 不要。コントラクト層を自分で厳密に定義する)

## 3. コマンド体系

既存の Slack 向け CLI の UX を踏襲しつつ Chatwork の語彙(room)に合わせる。

```
cw auth login [--name <alias>]        # トークンを対話入力(または --token-stdin)して検証・保存
cw auth list                          # 保存済みアカウント一覧
cw auth set-default <alias>
cw auth remove <alias>
cw auth status                        # /me を叩いて疎通確認

cw rooms list [--filter <substr>]     # チャット一覧(グループ/DM/マイチャット)
cw rooms get <room_id>
cw rooms members <room_id>            # メンション用に account_id を引くのに使う

cw messages read <room_id> [--limit N] [--force]   # メッセージ取得
cw messages send <room_id> <body>                  # 投稿(--file で本文をファイルから、- で stdin)
cw messages reply <room_id> <message_id> <body>    # [rp] + [To:] を自動付与した返信
cw messages get <room_id> <message_id>
cw messages edit <room_id> <message_id> <body>
cw messages delete <room_id> <message_id>
cw messages mark-read <room_id>

cw tasks list [--room <room_id>] [--status open|done]  # /my/tasks or /rooms/{id}/tasks
cw tasks create <room_id> --to <account_id>... --body <text> [--due <date>]
cw tasks done <room_id> <task_id>

cw files list <room_id>
cw files get <room_id> <file_id>      # ダウンロード URL 取得(または保存)

cw me                                 # 自分の情報
cw my status                          # 未読・タスク数

グローバルフラグ:
  --account <alias>    # 複数アカウントの切替(未指定ならデフォルト)
  --output json|table|text(既定: TTY なら table、パイプなら json)
  --quiet / --verbose
```

- 「複数チャンネル」は Chatwork では room。room はコマンド引数で都度指定するだけで管理不要
- 「複数認証ログイン」は `--account` で切替(Slack CLI の `--workspace` 相当)
- `messages reply` が要件の中核。`--mention` フラグで `[To:]` の有無を制御
  (Chatwork の返信記法 `[rp]` は通知されないため、既定で `[To:]` も併記して通知を飛ばす)

## 4. ローカル保存

既存の Slack 向け CLI(~/.config/<tool>/workspaces.json)に倣う。

- 場所: `os.UserConfigDir()/chatwork-cli/`(macOS: `~/Library/Application Support`、
  ただし `XDG_CONFIG_HOME` があれば `~/.config/chatwork-cli/` を優先)
- `accounts.json`:

```json
{
  "default": "work",
  "accounts": {
    "work": { "token": "...", "account_id": 123, "name": "田中" },
    "sub":  { "token": "...", "account_id": 456, "name": "個人" }
  }
}
```

- パーミッション `0600` を強制。login 時に chmod、読み込み時に緩ければ警告
- トークンは平文保存。macOS Keychain 対応は v2 の検討事項として残す
- 環境変数 `CHATWORK_API_TOKEN` があればファイルより優先(CI・agent のワンショット実行用)

## 5. アーキテクチャ

```
cmd/cw/main.go
internal/
  cli/         # cobra コマンド定義(薄く保つ。入出力の整形とバリデーションのみ)
  chatwork/    # API クライアント(コントラクト層)
    client.go  #   http.Client 注入可能。x-chatworktoken 付与、429 リトライ
    types.go   #   API レスポンスの型を厳密に定義
    message.go / room.go / task.go / file.go / me.go
  notation/    # メッセージ記法の組み立て([To:], [rp], [qt])。純粋関数
  config/      # accounts.json の読み書き
  output/      # json / table / text レンダラ
```

- `chatwork.Client` は `baseURL` を差し替え可能にし、`httptest.Server` でテスト
- 429 は `x-ratelimit-reset` を見て 1 回だけ自動リトライ(それでも 429 ならエラーで返す。
  暗黙に何度もリトライして agent を待たせない)
- エラーは「HTTP ステータス + Chatwork の errors 配列」をそのまま構造化して stderr / JSON に出す

## 6. AI agent フレンドリー(調査結果と方針)

「agent が使いやすい CLI」の条件を gh CLI 等の既存 CLI から整理:

1. **機械可読な出力**: `--output json` で全コマンドが安定したスキーマの JSON を返す。
   パイプ時は json を既定にして、agent がフラグを忘れても壊れない
2. **終了コードの規律**: 0=成功 / 1=API エラー / 2=使い方エラー。stderr にエラー、stdout に結果のみ
3. **自己記述性**: `cw --help` だけで全操作が発見できる。`cw docs --ai` で
   全コマンド仕様 + Chatwork 記法 + 典型ワークフローを 1 枚の Markdown として出力
   (agent がその場で読み込める)
4. **非対話で完結**: すべての入力をフラグ/stdin で渡せる。対話プロンプトは TTY のときだけ
5. **冪等・安全**: 破壊的操作(`delete`, `edit`)は `--yes` なしなら TTY で確認、
   非 TTY では明示フラグ必須
6. **指示書の同梱**: リポジトリに `.claude/skills/chatwork-cli/SKILL.md`(Claude 用 skill)と
   `AGENTS.md` を同梱。内容: コマンド早見表、`rooms members` で account_id を引いてから
   `messages reply` する手順、レート制限の注意
7. **ID をレスポンスに必ず含める**: message_id / room_id / account_id を JSON に含め、
   後続コマンドへそのまま渡せるようにする

## 7. テスト戦略

- **unit**: `notation/`(記法組み立て)、`config/`(読み書き・パーミッション)、
  `output/`(JSON スキーマの安定性)をテーブル駆動でカバー
- **API クライアント**: `httptest.Server` にゴールデンレスポンス(公式ドキュメントの例)を置き、
  リクエストヘッダー・パス・クエリの検証と、レスポンスのデコードを両方向で検証。
  429 リトライもここで検証
- **CLI 統合**: cobra コマンドを `ExecuteContext` で直接叩き、モックサーバー相手に
  stdout の JSON をスナップショット比較
- **E2E(手動/opt-in)**: `CHATWORK_API_TOKEN` がある時だけ動く `//go:build e2e` テストで
  マイチャットに投稿→取得→削除
- CI: GitHub Actions で `go test ./...` + `golangci-lint` + goreleaser の dry-run

## 8. 配布

- goreleaser: GitHub Releases に darwin/linux/windows × amd64/arm64 のバイナリ
- `go install github.com/kokoichi206/chatwork-cli/cmd/cw@latest`
- (必要になったら)Homebrew tap

## 9. マイルストーン

1. **M1**: auth(login/list/status) + rooms list + messages read/send + `--output json`
2. **M2**: messages reply(記法エンジン)+ rooms members + mark-read
3. **M3**: tasks / files / me / my status、edit・delete
4. **M4**: `cw docs --ai` + SKILL.md / AGENTS.md、goreleaser 配布

## 10. 未決事項(レビューで決めたい)

1. バイナリ名: `cw`(短い・タイポしにくい) vs `chatwork`(自己記述的)。推奨は `cw`
2. トークン保存: 平文 0600(シンプル)で v1 は行く、で OK か
3. `messages reply` の既定動作: `[rp]` + `[To:]` 併記(通知が飛ぶ)を既定にする、で OK か
4. Webhook 受信・ストリーミング(常駐)系は v1 スコープ外、で OK か
