# chatwork-cli

`cw` は Chatwork を CLI から操作するツールです。
人間と AI agent の両方が使うことを前提に設計しています。

- 個人 API トークンで **本人として** 読み書きする(bot ではない)
- 複数アカウントの管理(`--account` で切替)
- 全コマンドが `--output json` 対応(パイプ時は JSON が既定)
- Go 製シングルバイナリ

## インストール

```sh
go install github.com/kokoichi206/chatwork-cli/cmd/cw@latest
```

または [Releases](https://github.com/kokoichi206/chatwork-cli/releases) からバイナリを取得してください。

## セットアップ

```sh
cw auth guide                      # トークンの発行手順(画面のどこを開くか)を表示
cw auth login --name work          # 対話でトークン入力(手順も表示される)
echo "$TOKEN" | cw auth login --name work --token-stdin   # 非対話
cw auth status                     # 疎通確認
```

複数アカウントは `cw auth login --name <alias>` で追加し、
`cw auth set-default <alias>` か `--account <alias>` で切り替えます。
環境変数 `CHATWORK_API_TOKEN` があれば設定ファイルより優先されます(CI 向け)。

トークンは `~/.config/chatwork-cli/accounts.json`(なければ OS 標準の設定ディレクトリ)に
パーミッション 0600 で保存されます。

## 使い方

```sh
cw rooms list --filter "プロジェクト"          # チャット一覧
cw rooms members <room_id>                     # メンション用の account_id を調べる
cw messages read <room_id> --limit 20          # メッセージ取得
cw messages send <room_id> "こんにちは"        # 投稿(本人として)
cw messages reply <room_id> <message_id> "確認しました"   # 返信([rp]+[To:] を自動組み立て)
cw tasks list                                  # 自分のタスク
cw tasks create <room_id> --to <account_id> --body "レビュー" --due 2026-07-31
cw files list <room_id>
cw my status                                   # 未読・メンション数
```

全コマンドの一覧は `cw --help`、AI agent 向けの完全なリファレンスは `cw docs` で出力できます。

## リポジトリごとの部屋エイリアス

リポジトリに `.config/chatwork-cli.json` を置くと(カレントから親方向に探索)、
room_id を受け取るすべての箇所で部屋のエイリアス名が使えます。

```json
{
  "rooms": {
    "main": 405354226,
    "alerts": 123456789
  }
}
```

```sh
cw rooms aliases                 # このリポジトリに定義されたエイリアス一覧
cw messages send main "こんにちは"
cw tasks list --room alerts
```

ID とエイリアスだけの定義で個人差のある値(トークン・auth の alias 名)を含まないので、
コミットしてチームで共有できます。`.config/` 配下に置く形式は
[config-dir 提案](https://github.com/pi0/config-dir)の規約に従っています。

## AI agent から使う

```sh
cw agent init                # Claude Code の skill を ./.claude/skills/ に書き出す(コミットしてチーム共有)
cw agent init --scope user   # ~/.claude/skills/ に入れて全プロジェクトで使う
cw agent init --agents-md    # AGENTS.md に貼るスニペットを出力
```

- `cw docs` が自己完結の Markdown リファレンスを出力します(ワークフロー例・記法・注意点込み)。skill を入れなくても agent はこれで自己発見できます
- 出力は JSON(スキーマは Chatwork API v2 準拠 + ID を必ず含む)、終了コードは 0/1/2(成功/API エラー/使い方エラー)
- 破壊的操作(`messages edit|delete`, `auth remove`)は非対話環境では `--yes` が必須です

## 開発

```sh
go test ./...                                    # unit + 統合テスト(モックサーバー)
CHATWORK_API_TOKEN=... go test -tags e2e ./internal/chatwork/   # 実 API への E2E(マイチャットで完結)
go build ./cmd/cw
```

## 制約

- Chatwork API のレート制限: 全体 300 回/5 分、メッセージ投稿 10 回/10 秒。
  429 時はリセットまで(最大 15 秒)待って 1 回だけ自動リトライします
- メッセージ取得は API 仕様上、1 回につき最新 100 件までです
