package chatwork

// Chatwork API v2 のレスポンス型。
// フィールドは https://developer.chatwork.com/reference のスキーマと一致させること。

type Me struct {
	AccountID        int    `json:"account_id"`
	RoomID           int    `json:"room_id"`
	Name             string `json:"name"`
	ChatworkID       string `json:"chatwork_id"`
	OrganizationID   int    `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	Department       string `json:"department"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	Introduction     string `json:"introduction"`
	Mail             string `json:"mail"`
	AvatarImageURL   string `json:"avatar_image_url"`
	LoginMail        string `json:"login_mail"`
}

type MyStatus struct {
	UnreadRoomNum  int `json:"unread_room_num"`
	MentionRoomNum int `json:"mention_room_num"`
	MytaskRoomNum  int `json:"mytask_room_num"`
	UnreadNum      int `json:"unread_num"`
	MentionNum     int `json:"mention_num"`
	MytaskNum      int `json:"mytask_num"`
}

type Room struct {
	RoomID         int    `json:"room_id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Role           string `json:"role"`
	Sticky         bool   `json:"sticky"`
	UnreadNum      int    `json:"unread_num"`
	MentionNum     int    `json:"mention_num"`
	MytaskNum      int    `json:"mytask_num"`
	MessageNum     int    `json:"message_num"`
	FileNum        int    `json:"file_num"`
	TaskNum        int    `json:"task_num"`
	IconPath       string `json:"icon_path"`
	LastUpdateTime int64  `json:"last_update_time"`
	Description    string `json:"description,omitempty"`
}

type Member struct {
	AccountID        int    `json:"account_id"`
	Role             string `json:"role"`
	Name             string `json:"name"`
	ChatworkID       string `json:"chatwork_id"`
	OrganizationID   int    `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	Department       string `json:"department"`
	AvatarImageURL   string `json:"avatar_image_url"`
}

type Account struct {
	AccountID      int    `json:"account_id"`
	Name           string `json:"name"`
	AvatarImageURL string `json:"avatar_image_url"`
}

type Message struct {
	MessageID  string  `json:"message_id"`
	Account    Account `json:"account"`
	Body       string  `json:"body"`
	SendTime   int64   `json:"send_time"`
	UpdateTime int64   `json:"update_time"`
}

type ReadStatus struct {
	UnreadNum  int `json:"unread_num"`
	MentionNum int `json:"mention_num"`
}

type RoomSummary struct {
	RoomID   int    `json:"room_id"`
	Name     string `json:"name"`
	IconPath string `json:"icon_path"`
}

// MyTask は GET /my/tasks の要素。所属ルームを含む。
type MyTask struct {
	TaskID            int         `json:"task_id"`
	Room              RoomSummary `json:"room"`
	AssignedByAccount Account     `json:"assigned_by_account"`
	MessageID         string      `json:"message_id"`
	Body              string      `json:"body"`
	LimitTime         int64       `json:"limit_time"`
	Status            string      `json:"status"`
	LimitType         string      `json:"limit_type"`
}

// RoomTask は GET /rooms/{room_id}/tasks の要素。担当者を含む。
type RoomTask struct {
	TaskID            int     `json:"task_id"`
	Account           Account `json:"account"`
	AssignedByAccount Account `json:"assigned_by_account"`
	MessageID         string  `json:"message_id"`
	Body              string  `json:"body"`
	LimitTime         int64   `json:"limit_time"`
	Status            string  `json:"status"`
	LimitType         string  `json:"limit_type"`
}

type File struct {
	FileID      int     `json:"file_id"`
	Account     Account `json:"account"`
	MessageID   string  `json:"message_id"`
	Filename    string  `json:"filename"`
	Filesize    int64   `json:"filesize"`
	UploadTime  int64   `json:"upload_time"`
	DownloadURL string  `json:"download_url,omitempty"`
}
