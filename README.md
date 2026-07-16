# chatwork-cli

English | [日本語](README.ja.md)

`cw` is a command-line tool for [Chatwork](https://go.chatwork.com/), built for both humans and AI agents.

It uses your personal API token, so everything you read and post happens **as yourself** — no bot account, no special permissions. Point an AI agent at it and the agent can read rooms, reply with mentions, and manage tasks on your behalf.

- Read and post messages, reply with proper `[rp]`/`[To:]` notation (the author gets notified)
- Manage tasks and files, check unread/mention counts
- Multiple accounts, switchable with `--account`
- Every command speaks JSON (`--output json`, and by default when piped) — ideal for scripts and AI agents
- Single binary (Go), nothing else to install

## Install

```sh
go install github.com/kokoichi206/chatwork-cli/cmd/cw@latest
```

Or grab a binary from [Releases](https://github.com/kokoichi206/chatwork-cli/releases).

To update a release binary in place:

```sh
cw update            # replace the running binary with the latest release
cw update --dry-run  # only check whether a newer release exists
```

## Get started

```sh
cw auth guide              # shows where to issue your API token, step by step
cw auth login --name work  # paste the token (hidden input, validated immediately)
cw auth status             # confirms who you are
```

## Usage

```sh
cw rooms list --filter "project"            # find rooms
cw messages read <room_id> --limit 20       # read recent messages
cw messages send <room_id> "hello"          # post as yourself
cw messages reply <room_id> <message_id> "will do"   # reply; mentions the author
cw rooms members <room_id>                  # look up account_id for mentions
cw tasks list                               # your open tasks
cw my status                                # unread / mention counts
```

Run `cw --help` for all commands.

## Room aliases per repository

Put `.config/chatwork-cli.json` in a repository and its room aliases work anywhere a room_id is accepted. It contains only IDs and names, so commit it and share it with your team.

```json
{ "rooms": { "main": 405354226, "alerts": 123456789 } }
```

```sh
cw messages send main "hello"
cw rooms aliases
```

## For AI agents

```sh
cw agent init                # install the Claude Code skill into ./.claude/skills/
cw agent init --scope user   # or into ~/.claude/skills/ for all your projects
cw docs                      # self-contained Markdown reference an agent can read on the spot
```

Stable JSON schemas with IDs for command chaining, meaningful exit codes (0/1/2), and `--yes` guards on destructive commands in non-interactive mode.
