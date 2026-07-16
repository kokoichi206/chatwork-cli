package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// runCLIWithData は runCLI と同じだが、ローカル履歴の保存先を固定して呼び出し間で共有する。
func runCLIWithData(t *testing.T, handler http.Handler, opts cliOpts, dataDir string, args ...string) cliResult {
	t.Helper()
	opts.dataDir = dataDir
	return runCLI(t, handler, opts, args...)
}

func messagesHandler(t *testing.T, roomID int, pages ...string) (http.Handler, *int) {
	t.Helper()
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/rooms/%d/messages", roomID) {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("force") != "1" {
			t.Errorf("sync must fetch with force=1, got query %q", r.URL.RawQuery)
		}
		page := pages[min(call, len(pages)-1)]
		call++
		fmt.Fprint(w, page)
	})
	return handler, &call
}

func decodeSyncResult(t *testing.T, out string) map[string]any {
	t.Helper()
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	return res
}

func TestSyncAccumulatesAcrossRuns(t *testing.T) {
	dataDir := t.TempDir()
	first := `[
		{"message_id":"1","account":{"account_id":9,"name":"a"},"body":"m1","send_time":100},
		{"message_id":"2","account":{"account_id":9,"name":"a"},"body":"m2","send_time":200}
	]`
	// 2 回目は窓がずれて 1 が落ち、2 と新着 3 が重なる
	second := `[
		{"message_id":"2","account":{"account_id":9,"name":"a"},"body":"m2","send_time":200},
		{"message_id":"3","account":{"account_id":9,"name":"a"},"body":"m3","send_time":300}
	]`

	h1, _ := messagesHandler(t, 42, first)
	res := runCLIWithData(t, h1, cliOpts{cfg: singleAccount()}, dataDir, "sync", "42", "--output", "json")
	if res.err != nil {
		t.Fatalf("first sync: %v (stderr: %s)", res.err, res.stderr)
	}
	got := decodeSyncResult(t, res.stdout)
	if got["new"].(float64) != 2 || got["gap"].(bool) {
		t.Errorf("first sync result = %v", got)
	}

	h2, _ := messagesHandler(t, 42, second)
	res = runCLIWithData(t, h2, cliOpts{cfg: singleAccount()}, dataDir, "sync", "42", "--output", "json")
	if res.err != nil {
		t.Fatalf("second sync: %v (stderr: %s)", res.err, res.stderr)
	}
	got = decodeSyncResult(t, res.stdout)
	if got["new"].(float64) != 1 || got["gap"].(bool) {
		t.Errorf("second sync result = %v", got)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["message_id"] != "3" {
		t.Errorf("messages = %v", msgs)
	}

	// ローカル履歴には 1 が残っている(--local だけが 100 件の窓の外を読める)
	failOnAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("read --local must not call the API: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	res = runCLIWithData(t, failOnAPI, cliOpts{cfg: singleAccount()}, dataDir, "messages", "read", "42", "--local", "--output", "json")
	if res.err != nil {
		t.Fatalf("read --local: %v (stderr: %s)", res.err, res.stderr)
	}
	var local []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &local); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, res.stdout)
	}
	if len(local) != 3 || local[0]["message_id"] != "1" || local[2]["message_id"] != "3" {
		t.Errorf("local history = %v", local)
	}
}

// fullWindowJSON は messages API の 1 回分(100 件)のレスポンスを生成する。
func fullWindowJSON(startID int) string {
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf(`{"message_id":"%d","account":{"account_id":9,"name":"a"},"body":"m","send_time":%d}`, startID+i, startID+i)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestSyncReportsGap(t *testing.T) {
	dataDir := t.TempDir()
	first := `[{"message_id":"1","account":{"account_id":9,"name":"a"},"body":"m1","send_time":100}]`

	h1, _ := messagesHandler(t, 42, first)
	if res := runCLIWithData(t, h1, cliOpts{cfg: singleAccount()}, dataDir, "sync", "42"); res.err != nil {
		t.Fatal(res.err)
	}
	// 2 回目は窓いっぱい(100 件)でローカル最新(1)を含まない → 溢れの可能性
	h2, _ := messagesHandler(t, 42, fullWindowJSON(500))
	res := runCLIWithData(t, h2, cliOpts{cfg: singleAccount()}, dataDir, "sync", "42", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if got := decodeSyncResult(t, res.stdout); !got["gap"].(bool) {
		t.Errorf("gap not reported: %v", got)
	}
}

func TestReadLocalRejectsExplicitForce(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, "messages", "read", "42", "--local", "--force")
	if res.err == nil {
		t.Fatal("want usage error for --local with --force")
	}
}

func TestSyncAllIteratesRooms(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rooms":
			fmt.Fprint(w, `[{"room_id":1,"name":"a","type":"group"},{"room_id":2,"name":"b","type":"group"}]`)
		case "/rooms/1/messages", "/rooms/2/messages":
			fmt.Fprint(w, `[{"message_id":"10","account":{"account_id":9,"name":"a"},"body":"m","send_time":100}]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	res := runCLIWithData(t, handler, cliOpts{cfg: singleAccount()}, t.TempDir(), "sync", "--all", "--output", "json")
	if res.err != nil {
		t.Fatalf("sync --all: %v (stderr: %s)", res.err, res.stderr)
	}
	var results []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, res.stdout)
	}
	if len(results) != 2 || results[0]["room_id"].(float64) != 1 || results[1]["room_id"].(float64) != 2 {
		t.Errorf("results = %v", results)
	}
}

func TestSyncRequiresRoomOrAll(t *testing.T) {
	for _, args := range [][]string{{"sync"}, {"sync", "42", "--all"}} {
		res := runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, args...)
		if res.err == nil {
			t.Errorf("args %v: want usage error", args)
		}
	}
}

func TestReadLocalSinceFilters(t *testing.T) {
	dataDir := t.TempDir()
	// send_time: 2026-01-01 と 2026-07-01 (UTC 正午; ローカルタイムゾーンでも日付が変わらない時刻)
	page := `[
		{"message_id":"1","account":{"account_id":9,"name":"a"},"body":"old","send_time":1767268800},
		{"message_id":"2","account":{"account_id":9,"name":"a"},"body":"new","send_time":1782907200}
	]`
	h, _ := messagesHandler(t, 42, page)
	if res := runCLIWithData(t, h, cliOpts{cfg: singleAccount()}, dataDir, "sync", "42"); res.err != nil {
		t.Fatal(res.err)
	}
	res := runCLIWithData(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, dataDir,
		"messages", "read", "42", "--local", "--since", "2026-06-01", "--output", "json")
	if res.err != nil {
		t.Fatal(res.err)
	}
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &msgs); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, res.stdout)
	}
	if len(msgs) != 1 || msgs[0]["message_id"] != "2" {
		t.Errorf("msgs = %v", msgs)
	}
}

func TestReadSinceWithoutLocalIsUsageError(t *testing.T) {
	res := runCLI(t, http.NotFoundHandler(), cliOpts{cfg: singleAccount()}, "messages", "read", "42", "--since", "2026-06-01")
	if res.err == nil {
		t.Fatal("want usage error")
	}
}
