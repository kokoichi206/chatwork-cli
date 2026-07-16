package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/kokoichi206/chatwork-cli/internal/chatwork"
	"github.com/kokoichi206/chatwork-cli/internal/output"
	"github.com/kokoichi206/chatwork-cli/internal/store"
)

// syncResult は cw sync の JSON 出力。Messages は今回の新着分のみで、
// agent が「sync → 返り値の差分だけ判断」と書けるようにする。
type syncResult struct {
	RoomID   int                `json:"room_id"`
	New      int                `json:"new"`
	Updated  int                `json:"updated"`
	Gap      bool               `json:"gap"`
	Messages []chatwork.Message `json:"messages"`
}

func (a *app) syncCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "sync [room_id]",
		Short: "Fetch the latest messages and append new ones to local history",
		Long: `Fetch the latest 100 messages (force=1) and merge them into local history
(~/.local/share/chatwork-cli/<account_id>/rooms/<room_id>.jsonl, XDG_DATA_HOME respected).
Messages are deduped by message_id, so the server-side fetch cursor is never consumed.
"gap": true means the local history no longer overlaps the fetched window —
usually more than 100 messages were posted since the last sync, and the overflow
is permanently unavailable from the API. Read history with
'cw messages read <room_id> --local'.`,
		Args: rangeArgs(0, 1, "cw sync <room_id> | cw sync --all"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) == 1) {
				return usagef("pass exactly one of <room_id> or --all: cw sync <room_id> | cw sync --all")
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			accountID, err := a.storeAccountID(ctx)
			if err != nil {
				return err
			}
			dataDir, err := a.dataDir()
			if err != nil {
				return err
			}

			var roomIDs []int
			if all {
				rooms, err := client.Rooms(ctx)
				if err != nil {
					return err
				}
				for _, r := range rooms {
					roomIDs = append(roomIDs, r.RoomID)
				}
			} else {
				roomID, err := a.resolveRoom(args[0])
				if err != nil {
					return err
				}
				roomIDs = []int{roomID}
			}

			results := make([]syncResult, 0, len(roomIDs))
			for _, roomID := range roomIDs {
				res, err := a.syncRoom(ctx, client, dataDir, accountID, roomID)
				if err != nil {
					// 同期済みの部屋の書き込みは完了しているため、失敗した部屋から再実行すればよい
					return fmt.Errorf("sync room %d: %w", roomID, err)
				}
				results = append(results, res)
			}

			f, err := a.format()
			if err != nil {
				return err
			}
			switch f {
			case output.FormatJSON:
				if !all {
					return output.WriteJSON(a.deps.Stdout, results[0])
				}
				return output.WriteJSON(a.deps.Stdout, results)
			case output.FormatText:
				for _, r := range results {
					fmt.Fprintf(a.deps.Stdout, "synced: room_id=%d new=%d updated=%d gap=%t\n", r.RoomID, r.New, r.Updated, r.Gap)
				}
				return nil
			default:
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					rows = append(rows, []string{strconv.Itoa(r.RoomID), strconv.Itoa(r.New), strconv.Itoa(r.Updated), strconv.FormatBool(r.Gap)})
				}
				return output.WriteTable(a.deps.Stdout, []string{"ROOM_ID", "NEW", "UPDATED", "GAP"}, rows)
			}
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "sync every room you belong to (1 API request per room; the 300 req / 5 min limit applies)")
	return cmd
}

func (a *app) syncRoom(ctx context.Context, client *chatwork.Client, dataDir string, accountID, roomID int) (syncResult, error) {
	path := store.RoomPath(dataDir, accountID, roomID)
	// 取得も含めて部屋単位で直列化する。ロックなしでは並行 sync の Save が互いの新着を上書きする。
	unlock, err := store.Lock(path)
	if err != nil {
		return syncResult{}, err
	}
	defer unlock()

	fetched, err := client.Messages(ctx, roomID, true)
	if err != nil {
		return syncResult{}, err
	}
	local, err := store.Load(path)
	if err != nil {
		return syncResult{}, err
	}
	merged, res := store.Merge(local, fetched)
	if len(res.New) > 0 || res.Updated > 0 {
		if err := store.Save(path, merged); err != nil {
			return syncResult{}, err
		}
	}
	return syncResult{RoomID: roomID, New: len(res.New), Updated: res.Updated, Gap: res.Gap, Messages: res.New}, nil
}

// dataDir はローカル履歴のルートを返す。Deps.DataDir はテスト用の差し替え口。
func (a *app) dataDir() (string, error) {
	if a.deps.DataDir != "" {
		return a.deps.DataDir, nil
	}
	home := a.deps.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
	}
	return store.Dir(a.deps.Getenv, home), nil
}

// storeAccountID はローカル履歴のパスに使う account_id を解決する。
// 設定ファイルには login 時に /me で検証済みの account_id が保存されているためそれを使う。
// CHATWORK_API_TOKEN 経由のトークンは設定に紐づかないため /me を 1 回呼んで確認する
// (トークン文字列からは account が判別できず、取り違えると別アカウントの履歴に混入するため)。
func (a *app) storeAccountID(ctx context.Context) (int, error) {
	if a.accountFlag == "" && a.deps.Getenv(EnvToken) != "" {
		return a.accountIDFromMe(ctx)
	}
	cfg, _, err := a.loadConfig()
	if err != nil {
		return 0, err
	}
	_, acct, err := cfg.Resolve(a.accountFlag)
	if err != nil {
		return 0, err
	}
	if acct.AccountID == 0 { // 手編集などで account_id を欠く設定は /me で補う
		return a.accountIDFromMe(ctx)
	}
	return acct.AccountID, nil
}

func (a *app) accountIDFromMe(ctx context.Context) (int, error) {
	client, err := a.client()
	if err != nil {
		return 0, err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return 0, fmt.Errorf("resolve account_id via /me: %w", err)
	}
	return me.AccountID, nil
}
