//go:build e2e

// 実 API に対する opt-in テスト。自分のマイチャットに投稿し、取得・削除まで往復する。
// 実行: CHATWORK_API_TOKEN=... go test -tags e2e ./internal/chatwork/
package chatwork_test

import (
	"context"
	"os"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/chatwork"
)

func TestE2EMyChatRoundTrip(t *testing.T) {
	token := os.Getenv("CHATWORK_API_TOKEN")
	if token == "" {
		t.Skip("CHATWORK_API_TOKEN is not set")
	}
	ctx := context.Background()
	client := chatwork.New(token)

	me, err := client.Me(ctx)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	roomID := me.RoomID // マイチャット: 他人に影響しない

	messageID, err := client.PostMessage(ctx, roomID, "chatwork-cli e2e test", false)
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteMessage(ctx, roomID, messageID); err != nil {
			t.Errorf("cleanup DeleteMessage() error = %v", err)
		}
	})

	msg, err := client.Message(ctx, roomID, messageID)
	if err != nil {
		t.Fatalf("Message() error = %v", err)
	}
	if msg.Body != "chatwork-cli e2e test" {
		t.Errorf("body = %q", msg.Body)
	}
	if msg.Account.AccountID != me.AccountID {
		t.Errorf("posted as account_id=%d, want %d (must post as the token owner)", msg.Account.AccountID, me.AccountID)
	}
}
