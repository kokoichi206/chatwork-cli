package chatwork_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/kokoichi206/chatwork-cli/internal/chatwork"
)

func newTestClient(t *testing.T, handler http.Handler) *chatwork.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return chatwork.New("test-token", chatwork.WithBaseURL(srv.URL))
}

func TestRequestSendsTokenHeader(t *testing.T) {
	var gotToken, gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("x-chatworktoken")
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"account_id":1,"name":"田中"}`)
	}))

	me, err := client.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if gotToken != "test-token" {
		t.Errorf("x-chatworktoken = %q, want test-token", gotToken)
	}
	if gotPath != "/me" {
		t.Errorf("path = %q, want /me", gotPath)
	}
	if me.AccountID != 1 || me.Name != "田中" {
		t.Errorf("Me() = %+v", me)
	}
}

func TestPostMessageSendsForm(t *testing.T) {
	var gotContentType, gotBody, gotSelfUnread, gotMethod, gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotBody = r.PostForm.Get("body")
		gotSelfUnread = r.PostForm.Get("self_unread")
		fmt.Fprint(w, `{"message_id":"1234567890"}`)
	}))

	id, err := client.PostMessage(context.Background(), 42, "[To:1]\nこんにちは", true)
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}
	if id != "1234567890" {
		t.Errorf("message_id = %q", id)
	}
	if gotMethod != http.MethodPost || gotPath != "/rooms/42/messages" {
		t.Errorf("request = %s %s, want POST /rooms/42/messages", gotMethod, gotPath)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotBody != "[To:1]\nこんにちは" {
		t.Errorf("body = %q", gotBody)
	}
	if gotSelfUnread != "1" {
		t.Errorf("self_unread = %q, want 1", gotSelfUnread)
	}
}

func TestMessagesForceQueryAndDecoding(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("force") != "1" {
			t.Errorf("force = %q, want 1", r.URL.Query().Get("force"))
		}
		fmt.Fprint(w, `[{"message_id":"5","account":{"account_id":9,"name":"佐藤"},"body":"hi","send_time":1700000000,"update_time":0}]`)
	}))

	msgs, err := client.Messages(context.Background(), 7, true)
	if err != nil {
		t.Fatalf("Messages() error = %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "5" || msgs[0].Account.AccountID != 9 {
		t.Errorf("Messages() = %+v", msgs)
	}
}

func TestNoContentReturnsEmpty(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	msgs, err := client.Messages(context.Background(), 7, false)
	if err != nil {
		t.Fatalf("Messages() error = %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Messages() = %+v, want empty", msgs)
	}
}

func TestAPIErrorDecoding(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"errors":["Invalid API Token"]}`)
	}))

	_, err := client.Me(context.Background())
	var apiErr *chatwork.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	if len(apiErr.Errors) != 1 || apiErr.Errors[0] != "Invalid API Token" {
		t.Errorf("Errors = %v", apiErr.Errors)
	}
}

func TestRateLimitRetriesOnceAfterReset(t *testing.T) {
	now := time.Unix(1700000000, 0)
	var slept []time.Duration
	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("x-ratelimit-reset", strconv.FormatInt(now.Add(3*time.Second).Unix(), 10))
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"errors":["rate limit exceeded"]}`)
			return
		}
		fmt.Fprint(w, `{"account_id":1,"name":"n"}`)
	}))
	defer srv.Close()

	client := chatwork.New("t", chatwork.WithBaseURL(srv.URL),
		chatwork.WithClock(func() time.Time { return now }, func(d time.Duration) { slept = append(slept, d) }))

	if _, err := client.Me(context.Background()); err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if len(slept) != 1 || slept[0] != 3*time.Second {
		t.Errorf("slept = %v, want [3s]", slept)
	}
}

func TestRateLimitDoesNotRetryWhenResetIsTooFar(t *testing.T) {
	now := time.Unix(1700000000, 0)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("x-ratelimit-reset", strconv.FormatInt(now.Add(2*time.Minute).Unix(), 10))
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"errors":["rate limit exceeded"]}`)
	}))
	defer srv.Close()

	client := chatwork.New("t", chatwork.WithBaseURL(srv.URL),
		chatwork.WithClock(func() time.Time { return now }, func(time.Duration) { t.Error("should not sleep") }))

	_, err := client.Me(context.Background())
	var apiErr *chatwork.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("error = %v, want 429 APIError", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry)", calls)
	}
}

func TestRateLimitRetriesAtMostOnce(t *testing.T) {
	now := time.Unix(1700000000, 0)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("x-ratelimit-reset", strconv.FormatInt(now.Unix(), 10))
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"errors":["rate limit exceeded"]}`)
	}))
	defer srv.Close()

	client := chatwork.New("t", chatwork.WithBaseURL(srv.URL),
		chatwork.WithClock(func() time.Time { return now }, func(time.Duration) {}))

	_, err := client.Me(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// 429 待機中に context がキャンセルされたら、リトライせず即座に context エラーを返すこと。
func TestRateLimitWaitHonorsContextCancel(t *testing.T) {
	now := time.Unix(1700000000, 0)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("x-ratelimit-reset", strconv.FormatInt(now.Add(3*time.Second).Unix(), 10))
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"errors":["rate limit exceeded"]}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := chatwork.New("t", chatwork.WithBaseURL(srv.URL),
		chatwork.WithClock(func() time.Time { return now }, func(time.Duration) { cancel() }))

	_, err := client.Me(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry after cancel)", calls)
	}
}

// 204 No Content でもリスト取得は nil でなく空スライスを返すこと(JSON 出力が null にならない契約)。
func TestNoContentReturnsNonNilSlices(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.Background()

	msgs, err := client.Messages(ctx, 1, false)
	if err != nil || msgs == nil {
		t.Errorf("Messages() = %v, %v; want non-nil empty slice", msgs, err)
	}
	rooms, err := client.Rooms(ctx)
	if err != nil || rooms == nil {
		t.Errorf("Rooms() = %v, %v; want non-nil empty slice", rooms, err)
	}
	tasks, err := client.MyTasks(ctx, "open")
	if err != nil || tasks == nil {
		t.Errorf("MyTasks() = %v, %v; want non-nil empty slice", tasks, err)
	}
	files, err := client.Files(ctx, 1)
	if err != nil || files == nil {
		t.Errorf("Files() = %v, %v; want non-nil empty slice", files, err)
	}
}

func TestCreateTaskForm(t *testing.T) {
	var gotToIDs, gotLimit, gotLimitType string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotToIDs = r.PostForm.Get("to_ids")
		gotLimit = r.PostForm.Get("limit")
		gotLimitType = r.PostForm.Get("limit_type")
		fmt.Fprint(w, `{"task_ids":[123,124]}`)
	}))

	ids, err := client.CreateTask(context.Background(), 1, "レビュー", []int{10, 20}, 1753887600, "date")
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != 123 {
		t.Errorf("task_ids = %v", ids)
	}
	if gotToIDs != "10,20" {
		t.Errorf("to_ids = %q, want 10,20", gotToIDs)
	}
	if gotLimit != "1753887600" || gotLimitType != "date" {
		t.Errorf("limit = %q, limit_type = %q", gotLimit, gotLimitType)
	}
}
