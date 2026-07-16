package chatwork

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.chatwork.com/v2"

// maxRetryWait は 429 自動リトライで待つ時間の上限。
// 全体制限(5分窓)のリセットまで黙って待つと呼び出し元を長時間ブロックするため、
// これを超える待ちはリトライせずエラーで返す。
const maxRetryWait = 15 * time.Second

type Client struct {
	baseURL string
	token   string
	hc      *http.Client
	now     func() time.Time
	sleep   func(time.Duration)
}

type Option func(*Client)

func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.hc = hc }
}

func WithClock(now func() time.Time, sleep func(time.Duration)) Option {
	return func(c *Client) {
		c.now = now
		c.sleep = sleep
	}
}

func New(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		token:   token,
		hc:      &http.Client{Timeout: 30 * time.Second},
		now:     time.Now,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// sleepUntil は context キャンセルで中断できる待機。
// キャンセルされた場合はリトライせず即座に呼び出し元へ返すための error を返す。
func (c *Client) sleepUntil(ctx context.Context, d time.Duration) error {
	if c.sleep != nil { // テスト注入時も ctx キャンセルの伝播は保証する
		c.sleep(d)
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// APIError は 4xx/5xx レスポンス。Errors は Chatwork API の errors 配列そのまま。
type APIError struct {
	StatusCode int      `json:"status_code"`
	Errors     []string `json:"errors"`
}

func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("chatwork api: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("chatwork api: HTTP %d: %s", e.StatusCode, strings.Join(e.Errors, "; "))
}

// do はリクエストを 1 回実行し、429 の場合のみ x-ratelimit-reset まで待って 1 回だけ再試行する。
// 204 No Content は out を書き換えず正常終了として扱う(メッセージ 0 件などで返る)。
func (c *Client) do(ctx context.Context, method, path string, query, form url.Values, out any) error {
	for attempt := 0; ; attempt++ {
		retryable, err := c.doOnce(ctx, method, path, query, form, out)
		if err == nil {
			return nil
		}
		if !retryable || attempt >= 1 {
			return err
		}
	}
}

func (c *Client) doOnce(ctx context.Context, method, path string, query, form url.Values, out any) (retryable bool, err error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return false, err
	}
	req.Header.Set("x-chatworktoken", c.token)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNoContent:
		return false, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		apiErr := decodeAPIError(resp)
		wait, ok := waitUntilReset(resp.Header.Get("x-ratelimit-reset"), c.now())
		if ok && wait <= maxRetryWait {
			if err := c.sleepUntil(ctx, wait); err != nil {
				return false, err
			}
			return true, apiErr
		}
		return false, apiErr
	case resp.StatusCode >= 400:
		return false, decodeAPIError(resp)
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return false, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, fmt.Errorf("chatwork api: decode response for %s %s: %w", method, path, err)
	}
	return false, nil
}

func decodeAPIError(resp *http.Response) *APIError {
	apiErr := &APIError{StatusCode: resp.StatusCode}
	var payload struct {
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
		apiErr.Errors = payload.Errors
	}
	return apiErr
}

// waitUntilReset は x-ratelimit-reset(unix 秒)ヘッダーから待ち時間を計算する。
// ヘッダーが欠落・不正な場合は ok=false でリトライしない。
func waitUntilReset(header string, now time.Time) (wait time.Duration, ok bool) {
	reset, err := strconv.ParseInt(header, 10, 64)
	if err != nil {
		return 0, false
	}
	wait = max(time.Unix(reset, 0).Sub(now), 0)
	return wait, true
}
