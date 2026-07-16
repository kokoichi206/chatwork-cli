# chatwork-cli

[English](README.md) | 日本語

`cw` は [Chatwork](https://go.chatwork.com/) を CLI から操作するツールです。人間と AI agent の両方で使うことを前提に設計しています。

個人 API トークンを使うため、読み書きはすべて**あなた本人として**行われます。bot アカウントも特別な権限も不要で、AI agent に持たせればあなたの代わりに会話の確認・メンション付き返信・タスク管理ができます。

- メッセージの取得・投稿、`[rp]`/`[To:]` 記法での返信(相手に通知が届く)
- タスク・ファイルの管理、未読・メンション数の確認
- 複数アカウントを `--account` で切替
- 全コマンドが JSON 対応(`--output json`、パイプ時は既定で JSON) — スクリプトや AI agent に最適
- Go 製シングルバイナリ、追加のランタイム不要

## インストール

```sh
go install github.com/kokoichi206/chatwork-cli/cmd/cw@latest
```

または [Releases](https://github.com/kokoichi206/chatwork-cli/releases) からバイナリを取得してください。

リリースバイナリはその場で更新できます。

```sh
cw update            # 実行中のバイナリを最新リリースに置き換える
cw update --dry-run  # 新しいリリースがあるかの確認のみ
```

## はじめる

```sh
cw auth guide              # トークンの発行手順(どの画面を開くか)を表示
cw auth login --name work  # トークンを貼り付け(非表示入力・即時検証)
cw auth status             # 本人確認
```

## 使い方

```sh
cw rooms list --filter "プロジェクト"        # 部屋を探す
cw messages read <room_id> --limit 20        # 直近のメッセージを読む
cw messages send <room_id> "こんにちは"      # 本人として投稿
cw messages reply <room_id> <message_id> "対応します"   # 返信(相手にメンション通知)
cw rooms members <room_id>                   # メンション用の account_id を調べる
cw tasks list                                # 自分のタスク
cw my status                                 # 未読・メンション数
```

全コマンドは `cw --help` で確認できます。

## リポジトリごとの部屋エイリアス

リポジトリに `.config/chatwork-cli.json` を置くと、room_id を受け取るすべての箇所でエイリアス名が使えます。ID と名前だけの定義なので、コミットしてチームで共有できます。

```json
{ "rooms": { "main": 405354226, "alerts": 123456789 } }
```

```sh
cw messages send main "こんにちは"
cw rooms aliases
```

## AI agent から使う

```sh
cw agent init                # Claude Code の skill を ./.claude/skills/ に書き出す
cw agent init --scope user   # ~/.claude/skills/ に入れて全プロジェクトで使う
cw docs                      # agent がその場で読める自己完結の Markdown リファレンス
```

ID を必ず含む安定した JSON スキーマ、意味のある終了コード(0/1/2)、非対話環境での破壊的操作には `--yes` ガード付きです。
