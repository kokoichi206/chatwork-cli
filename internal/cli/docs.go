package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *app) docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the full AI-agent oriented reference as Markdown",
		Args:  exactArgs(0, "cw docs"),
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Fprint(a.deps.Stdout, agentDocs)
			return nil
		},
	}
}

// agentDocs は agent がその場で読み込む前提の自己完結リファレンス。
// コマンド仕様を変えたらここも更新すること(cli パッケージのテストで主要コマンドの存在を検証している)。
const agentDocs = `# cw — Chatwork CLI reference (for AI agents)

cw operates Chatwork as the authenticated human user (personal API token).
Everything you post appears as that user's own message.

## Conventions

- Output: pass ` + "`--output json`" + ` (or pipe stdout; piped output defaults to JSON).
  JSON mirrors the Chatwork API v2 schema and always contains IDs (room_id / message_id / account_id / task_id) for chaining commands.
  Exception: documentation commands (` + "`cw docs`" + `, ` + "`cw auth guide`" + `) always output Markdown text regardless of --output.
  List commands return ` + "`[]`" + ` (never null) when there are no items.
- Exit codes: 0 = success, 1 = API/runtime error, 2 = usage error. Errors go to stderr as text.
- Destructive commands (messages edit/delete, auth remove) require ` + "`--yes`" + ` when not on a TTY.
- Accounts: ` + "`--account <alias>`" + ` switches between logged-in accounts. The env var CHATWORK_API_TOKEN overrides the config file when --account is not given.
- Rate limits (server side): 300 requests / 5 min overall; message posting 10 / 10 sec. On 429 cw waits for the reset (max 15s) and retries once, then fails.

## Setup

    cw auth guide                      # step-by-step: where to issue a token on the Chatwork screen
    echo "$TOKEN" | cw auth login --name work --token-stdin
    cw auth status                     # verify: prints your name and account_id
    cw auth list                       # list aliases; * marks the default
    cw auth set-default work

To make this reference permanently discoverable, install the bundled Claude Code skill:

    cw agent init                      # writes ./.claude/skills/chatwork-cli/SKILL.md (commit it)
    cw agent init --scope user         # or ~/.claude/skills/chatwork-cli/SKILL.md
    cw agent init --agents-md          # print a snippet to paste into AGENTS.md

Release binaries can update themselves:

    cw update                          # replace the running binary with the latest GitHub release
    cw update --dry-run                # only check whether a newer release exists

## Project config (room aliases)

If the repository has ` + "`.config/chatwork-cli.json`" + ` (searched upward from the current directory), its room aliases can be used anywhere a room_id is accepted:

    {"rooms": {"main": 405354226, "alerts": 123456789}}

    cw rooms aliases --output json     # discover this repo's rooms — check this FIRST
    cw messages send main "こんにちは"  # alias instead of room_id

The file defines rooms only (safe to commit; no tokens, no per-person values).
Account selection is personal: --account flag > CHATWORK_API_TOKEN > the default from ` + "`cw auth set-default`" + `.

## Typical workflows

### Find a room and read recent messages

    cw rooms list --filter "プロジェクト" --output json
    cw messages read <room_id> --limit 20 --output json
    # each message: {"message_id","account":{"account_id","name"},"body","send_time"}

### Reply to a message with a mention (the core workflow)

    cw messages reply <room_id> <message_id> "確認しました" --output json
    # Builds "[rp aid=<author> to=<room>-<message>][To:<author>]\n確認しました".
    # The author gets a notification. Add --no-mention to skip the [To:] tag.

### Post a new message mentioning someone

    cw rooms members <room_id> --output json      # find the target account_id
    cw messages send <room_id> "[To:123456]お願いします" --output json
    # returns {"room_id":..., "message_id":"..."}

### Multiline / long bodies

    cw messages send <room_id> - <<'EOF'
    1行目
    2行目
    EOF
    # or: cw messages send <room_id> --file body.txt

### Tasks

    cw tasks list --output json                            # your open tasks
    cw tasks list --room <room_id> --status open
    cw tasks create <room_id> --to <account_id> --body "レビュー対応" --due 2026-07-31
    cw tasks done <room_id> <task_id>

### Files

    cw files list <room_id> --output json
    cw files get <room_id> <file_id> --output json   # download_url expires in ~30 seconds; fetch it immediately

### Unread management

    cw my status --output json          # unread/mention/task counts
    cw messages mark-read <room_id>

## Chatwork message notation (used inside message bodies)

- Mention: [To:ACCOUNT_ID] — notifies the user. Get IDs from ` + "`cw rooms members`" + `.
- Reply: [rp aid=ACCOUNT_ID to=ROOM_ID-MESSAGE_ID] — shows the RE link; does NOT notify by itself.
- Quote: [qt][qtmeta aid=ACCOUNT_ID time=UNIX_SECONDS]quoted text[/qt]
- Box: [info][title]title[/title]body[/info], horizontal rule: [hr]

## Cautions

- ` + "`messages read --force=false`" + ` returns only messages not fetched before with this token (may be empty). Default is --force (latest 100).
- message_id is a string; keep it quoted.
- Posting is rate-limited to 10 per 10 seconds — batch content into one message instead of many small ones.
`
