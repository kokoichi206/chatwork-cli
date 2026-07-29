package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// tokenGuideShort は対話ログイン時にプロンプトの前に表示する最小手順。
const tokenGuideShort = `Chatwork API トークンの取得方法:
  1. https://www.chatwork.com/service/packages/chatwork/subpackages/api/token.php を開く
  2. パスワードを入力して「表示」→ トークンをコピー
     (詳細は cw auth guide)
`

// tokenGuideFull は cw auth guide の出力。
// 手順の出典: https://developer.chatwork.com/docs/getting-started
const tokenGuideFull = `# Chatwork API トークンの取得と登録

## 1. トークンを発行する

1. API Token ページを開く:
   https://www.chatwork.com/service/packages/chatwork/subpackages/api/token.php
   (画面からたどる場合は右上の「利用者名」→「サービス連携」→「API Token」)
2. ログインパスワードを入力して「表示」をクリックする
3. 表示されたトークン文字列を「コピー」する

ビジネス / エンタープライズプランでは組織管理者の承認が必要です。
トークンが表示されない場合は「API利用申請」から申請してください。
申請ページ: https://www.chatwork.com/service/packages/chatwork/subpackages/api/request.php
(パーソナル / フリープランでは申請は不要です)

## 2. トークンを cw に登録する

対話で貼り付ける場合(入力はローカルの設定ファイルにのみ保存されます):

    cw auth login --name work

非対話(スクリプトや agent 経由)の場合:

    echo "$TOKEN" | cw auth login --name work --token-stdin

--name は好きなエイリアスです(例: work, personal)。登録時に GET /me で疎通確認され、
成功するとアカウント名と account_id が表示されます。

## 3. 確認と切り替え

    cw auth status              # いま使われるトークンで /me を叩いて確認
    cw auth list                # 登録済みアカウント一覧(* がデフォルト)
    cw auth set-default work    # デフォルトを切り替え
    cw rooms list --account sub # コマンド単位で切り替え

## 保存場所とセキュリティ

- 保存先: ~/.config/chatwork-cli/accounts.json (XDG_CONFIG_HOME があればそちら)。
  パーミッション 0600 で保存されます
- トークンはあなた本人の全権限(閲覧・投稿・削除)を持ちます。第三者に共有しないでください
- ファイルに保存したくない場合は環境変数 CHATWORK_API_TOKEN でも動作します:

    CHATWORK_API_TOKEN="$TOKEN" cw rooms list
`

func (a *app) authGuideCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "guide",
		Short: "Show step-by-step instructions to get and register an API token",
		Args:  exactArgs(0, "cw auth guide"),
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Fprint(a.deps.Stdout, tokenGuideFull)
			return nil
		},
	}
}
