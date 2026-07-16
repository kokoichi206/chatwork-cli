# chatwork-cli — agent guide

This repo builds `cw`, a Chatwork CLI for humans and AI agents.

## Using the built CLI

Run `cw docs` for the complete self-contained reference (workflows, notation, cautions).
Quick rules:

- Always pass `--output json`. Responses mirror Chatwork API v2 and include IDs for chaining.
- Exit codes: 0 = success, 1 = API error, 2 = usage error.
- Destructive commands (`messages edit|delete`, `auth remove`) need `--yes` in non-interactive mode.
- Find `account_id` for mentions via `cw rooms members <room_id>`; then `cw messages reply` handles `[rp]`/`[To:]` notation for you.
- Rate limits: posting is 10 requests / 10 sec — prefer one combined message over many small ones.

## Working on this codebase

- Layout: `internal/chatwork` (API contract layer, no CLI concerns) / `internal/cli` (thin cobra layer) /
  `internal/notation` (message notation, pure functions) / `internal/config` / `internal/output`.
- API types in `internal/chatwork/types.go` must match https://developer.chatwork.com/reference exactly.
- Tests: `go test ./...` uses `httptest` mock servers only; never hits the real API.
  Real-API E2E is opt-in: `CHATWORK_API_TOKEN=... go test -tags e2e ./internal/chatwork/`.
- If you change command behavior, update the embedded reference in `internal/cli/docs.go`
  and the skill. The canonical skill is `internal/cli/SKILL.md` (embedded into the binary for
  `cw agent init`); `.claude/skills/chatwork-cli/SKILL.md` must be an identical copy
  (enforced by TestRepoSkillMatchesEmbedded).
