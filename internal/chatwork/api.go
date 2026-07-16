package chatwork

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) Me(ctx context.Context) (*Me, error) {
	var me Me
	if err := c.do(ctx, http.MethodGet, "/me", nil, nil, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

func (c *Client) MyStatus(ctx context.Context) (*MyStatus, error) {
	var st MyStatus
	if err := c.do(ctx, http.MethodGet, "/my/status", nil, nil, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// MyTasks は自分に割り当てられたタスクを取得する。status は "open" / "done" / 空(両方)。
func (c *Client) MyTasks(ctx context.Context, status string) ([]MyTask, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	var tasks []MyTask
	if err := c.do(ctx, http.MethodGet, "/my/tasks", q, nil, &tasks); err != nil {
		return nil, err
	}
	if tasks == nil { // 204 No Content: JSON 出力を null でなく [] にするため空スライスへ正規化
		tasks = []MyTask{}
	}
	return tasks, nil
}

func (c *Client) Rooms(ctx context.Context) ([]Room, error) {
	var rooms []Room
	if err := c.do(ctx, http.MethodGet, "/rooms", nil, nil, &rooms); err != nil {
		return nil, err
	}
	if rooms == nil {
		rooms = []Room{}
	}
	return rooms, nil
}

func (c *Client) Room(ctx context.Context, roomID int) (*Room, error) {
	var room Room
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d", roomID), nil, nil, &room); err != nil {
		return nil, err
	}
	return &room, nil
}

func (c *Client) Members(ctx context.Context, roomID int) ([]Member, error) {
	var members []Member
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d/members", roomID), nil, nil, &members); err != nil {
		return nil, err
	}
	if members == nil {
		members = []Member{}
	}
	return members, nil
}

// Messages は最大 100 件のメッセージを取得する。
// force=false では前回取得分からの差分のみが返り、差分がなければ空スライスになる(API は 204)。
func (c *Client) Messages(ctx context.Context, roomID int, force bool) ([]Message, error) {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	var msgs []Message
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d/messages", roomID), q, nil, &msgs); err != nil {
		return nil, err
	}
	if msgs == nil { // 差分なしは 204 No Content で返る
		msgs = []Message{}
	}
	return msgs, nil
}

func (c *Client) PostMessage(ctx context.Context, roomID int, body string, selfUnread bool) (string, error) {
	form := url.Values{"body": {body}}
	if selfUnread {
		form.Set("self_unread", "1")
	}
	var res struct {
		MessageID string `json:"message_id"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/rooms/%d/messages", roomID), nil, form, &res); err != nil {
		return "", err
	}
	return res.MessageID, nil
}

func (c *Client) Message(ctx context.Context, roomID int, messageID string) (*Message, error) {
	var msg Message
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d/messages/%s", roomID, url.PathEscape(messageID)), nil, nil, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) EditMessage(ctx context.Context, roomID int, messageID, body string) error {
	form := url.Values{"body": {body}}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/rooms/%d/messages/%s", roomID, url.PathEscape(messageID)), nil, form, nil)
}

func (c *Client) DeleteMessage(ctx context.Context, roomID int, messageID string) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/rooms/%d/messages/%s", roomID, url.PathEscape(messageID)), nil, nil, nil)
}

// MarkRead は messageID まで(空なら全件)を既読にする。
func (c *Client) MarkRead(ctx context.Context, roomID int, messageID string) (*ReadStatus, error) {
	form := url.Values{}
	if messageID != "" {
		form.Set("message_id", messageID)
	}
	var st ReadStatus
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/rooms/%d/messages/read", roomID), nil, form, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (c *Client) RoomTasks(ctx context.Context, roomID int, status string) ([]RoomTask, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	var tasks []RoomTask
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d/tasks", roomID), q, nil, &tasks); err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []RoomTask{}
	}
	return tasks, nil
}

// CreateTask は担当者 toIDs にタスクを作成する。limit は期限(unix 秒)で 0 なら期限なし。
// limitType は "date" / "time" / 空(API 既定)。
func (c *Client) CreateTask(ctx context.Context, roomID int, body string, toIDs []int, limit int64, limitType string) ([]int, error) {
	ids := make([]string, len(toIDs))
	for i, id := range toIDs {
		ids[i] = strconv.Itoa(id)
	}
	form := url.Values{
		"body":   {body},
		"to_ids": {strings.Join(ids, ",")},
	}
	if limit > 0 {
		form.Set("limit", strconv.FormatInt(limit, 10))
	}
	if limitType != "" {
		form.Set("limit_type", limitType)
	}
	var res struct {
		TaskIDs []int `json:"task_ids"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/rooms/%d/tasks", roomID), nil, form, &res); err != nil {
		return nil, err
	}
	return res.TaskIDs, nil
}

// UpdateTaskStatus は status に "open" または "done" を指定する。
func (c *Client) UpdateTaskStatus(ctx context.Context, roomID, taskID int, status string) error {
	form := url.Values{"body": {status}}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/rooms/%d/tasks/%d/status", roomID, taskID), nil, form, nil)
}

func (c *Client) Files(ctx context.Context, roomID int) ([]File, error) {
	var files []File
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d/files", roomID), nil, nil, &files); err != nil {
		return nil, err
	}
	if files == nil {
		files = []File{}
	}
	return files, nil
}

// File は createDownloadURL=true のとき 30 秒間有効なダウンロード URL を含めて返す。
func (c *Client) File(ctx context.Context, roomID, fileID int, createDownloadURL bool) (*File, error) {
	q := url.Values{}
	if createDownloadURL {
		q.Set("create_download_url", "1")
	}
	var f File
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rooms/%d/files/%d", roomID, fileID), q, nil, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
