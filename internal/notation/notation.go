// Package notation は Chatwork のメッセージ記法を組み立てる純粋関数群。
// 仕様: https://developer.chatwork.com/docs/message-notation
package notation

import (
	"fmt"
	"strings"
)

func Mention(accountID int) string {
	return fmt.Sprintf("[To:%d]", accountID)
}

func ReplyTag(accountID, roomID int, messageID string) string {
	return fmt.Sprintf("[rp aid=%d to=%d-%s]", accountID, roomID, messageID)
}

func Quote(accountID int, sendTime int64, body string) string {
	return fmt.Sprintf("[qt][qtmeta aid=%d time=%d]%s[/qt]", accountID, sendTime, body)
}

func Info(title, body string) string {
	if title == "" {
		return fmt.Sprintf("[info]%s[/info]", body)
	}
	return fmt.Sprintf("[info][title]%s[/title]%s[/info]", title, body)
}

// ReplyBody は返信メッセージの本文全体を組み立てる。
// [rp] 記法だけでは相手に通知が飛ばないため、withMention=true で [To:] を併記して未読通知を立てる。
func ReplyBody(accountID, roomID int, messageID, body string, withMention bool) string {
	var b strings.Builder
	b.WriteString(ReplyTag(accountID, roomID, messageID))
	if withMention {
		b.WriteString(Mention(accountID))
	}
	b.WriteString("\n")
	b.WriteString(body)
	return b.String()
}
