---
name: chatwork-cli
description: Operate Chatwork (read rooms, post/reply to messages with mentions, manage tasks/files) via the `cw` CLI as the authenticated user. Use when asked to read or send Chatwork messages, reply with a mention, or manage Chatwork tasks.
---

# Chatwork CLI (`cw`)

`cw` posts as the authenticated human user (personal API token) — everything you send appears as that user's own message. Be careful with wording; you are speaking as them.

For the complete reference, run:

    cw docs

## Essentials

- Always pass `--output json`; results include `room_id` / `message_id` / `account_id` / `task_id` for chaining.
- Exit codes: 0 success, 1 API error, 2 usage error. Error text goes to stderr.
- Destructive commands (`messages edit|delete`, `auth remove`) require `--yes` in non-interactive mode.
- Switch accounts with `--account <alias>` (`cw auth list` shows aliases).
- If no account is configured, tell the user to run `cw auth guide` — it walks through issuing a token on the Chatwork screen and saving it.

## Core workflows

Check the repo's room aliases first (defined in `.config/chatwork-cli.json`, usable anywhere a room_id is accepted):

    cw rooms aliases --output json
    cw messages send main "本文" --output json    # alias instead of room_id

Find a room, read messages:

    cw rooms list --filter "<name substring>" --output json
    cw messages read <room_id> --limit 20 --output json

Reply with a mention (default; the author is notified):

    cw messages reply <room_id> <message_id> "本文" --output json

New message mentioning someone (look up the account_id first):

    cw rooms members <room_id> --output json
    cw messages send <room_id> "[To:<account_id>]本文" --output json

Multiline body via stdin:

    cw messages send <room_id> - <<'EOF'
    ...
    EOF

Read past the latest 100 messages (the API has no pagination; `cw sync` accumulates local history):

    cw sync <room_id> --output json               # returns only the newly seen messages
    cw messages read <room_id> --local --since YYYY-MM-DD --output json

Tasks:

    cw tasks list --output json
    cw tasks create <room_id> --to <account_id> --body "..." --due YYYY-MM-DD
    cw tasks done <room_id> <task_id>

## Cautions

- Posting is rate-limited (10 / 10 sec): send one combined message instead of many small ones.
- `message_id` is a string — keep it quoted in scripts.
- `cw sync` reporting `"gap": true` means messages likely overflowed the 100-message window since the last sync and are permanently unavailable — sync frequently in busy rooms.
- Never echo or log the API token; `cw auth list` output is safe (tokens are excluded).
