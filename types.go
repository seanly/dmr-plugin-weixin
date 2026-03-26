package main

import "strings"

// JSON shapes for ilink/bot API (reference: openclaw-weixin src/api/types.ts).

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

type getUpdatesReq struct {
	GetUpdatesBuf string   `json:"get_updates_buf,omitempty"`
	BaseInfo      baseInfo `json:"base_info"`
}

type getUpdatesResp struct {
	Ret                  int             `json:"ret"`
	Errcode              int             `json:"errcode"`
	Errmsg               string          `json:"errmsg"`
	Msgs                 []weixinMessage `json:"msgs"`
	GetUpdatesBuf        string          `json:"get_updates_buf"`
	LongpollingTimeoutMs int             `json:"longpolling_timeout_ms"`
}

type textItem struct {
	Text string `json:"text,omitempty"`
}

type cdnMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AesKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

type imageItem struct {
	Media      *cdnMedia `json:"media,omitempty"`
	ThumbMedia *cdnMedia `json:"thumb_media,omitempty"`
	// AesKeyHex: raw AES-128 key as hex string (16 bytes); preferred over media.aes_key for inbound decryption.
	AesKeyHex string `json:"aeskey,omitempty"`
	URL       string `json:"url,omitempty"`
	MidSize   int    `json:"mid_size,omitempty"`
	// HdSize: ciphertext length for full image; some Weixin clients need this to open preview (see openclaw upload.ts comment).
	HdSize int `json:"hd_size,omitempty"`
	// ThumbSize / thumb_*: mobile Weixin often shows a grey box if thumb fields are missing when no_need_thumb was used on upload.
	ThumbSize   int `json:"thumb_size,omitempty"`
	ThumbWidth  int `json:"thumb_width,omitempty"`
	ThumbHeight int `json:"thumb_height,omitempty"`
}

type voiceItem struct {
	Media *cdnMedia `json:"media,omitempty"`
	Text  string    `json:"text,omitempty"`
}

type fileItem struct {
	Media    *cdnMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	Len      string    `json:"len,omitempty"`
	MD5      string    `json:"md5,omitempty"` // optional; OpenClaw omits — leave empty for strict parity
}

type videoItem struct {
	Media      *cdnMedia `json:"media,omitempty"`
	ThumbMedia *cdnMedia `json:"thumb_media,omitempty"`
	VideoSize  int       `json:"video_size,omitempty"`
}

type refMessage struct {
	MessageItem *messageItem `json:"message_item,omitempty"`
	Title       string       `json:"title,omitempty"`
	SvrID       int64        `json:"svrid,omitempty"`
	MessageID   int64        `json:"message_id,omitempty"`
	NewMsgID    int64        `json:"new_msg_id,omitempty"`
	CreateTime  int64        `json:"create_time,omitempty"`
	FromUserID  string       `json:"from_user_id,omitempty"`
}

type messageItem struct {
	Type      int         `json:"type,omitempty"`
	TextItem  *textItem   `json:"text_item,omitempty"`
	ImageItem *imageItem  `json:"image_item,omitempty"`
	VoiceItem *voiceItem  `json:"voice_item,omitempty"`
	FileItem  *fileItem   `json:"file_item,omitempty"`
	VideoItem *videoItem  `json:"video_item,omitempty"`
	RefMsg    *refMessage `json:"ref_msg,omitempty"`
}

type weixinMessage struct {
	Seq       int64 `json:"seq,omitempty"`
	MessageID int64 `json:"message_id,omitempty"`
	// Outbound bot sends use empty string; keep key in JSON (OpenClaw passes from_user_id: "").
	FromUserID string `json:"from_user_id"`
	ToUserID   string `json:"to_user_id,omitempty"`
	ClientID   string `json:"client_id,omitempty"`
	// Always serialized on outbound (no omitempty) so gateways see explicit "session_id":"…" even when empty.
	SessionID sessionIDJSON `json:"session_id"`
	// Some gateways use camelCase / PascalCase; json does not alias these to session_id.
	SessionIDCamel   string        `json:"sessionId,omitempty"`
	SessionIDPascal  string        `json:"SessionId,omitempty"`
	MlinkSessionID   string        `json:"mlink_session_id,omitempty"`
	IlinkSessionID   string        `json:"ilink_session_id,omitempty"`
	ConversationID   string        `json:"conversation_id,omitempty"`
	ConvID           string        `json:"conv_id,omitempty"`
	ChatSessionID    string        `json:"chat_session_id,omitempty"`
	GroupID          string        `json:"group_id,omitempty"`
	MessageType      int           `json:"message_type,omitempty"`
	MessageState     int           `json:"message_state,omitempty"`
	ItemList         []messageItem `json:"item_list,omitempty"`
	ContextToken     string        `json:"context_token,omitempty"`
	ContextTokCamel  string        `json:"contextToken,omitempty"`
	ContextTokPascal string        `json:"ContextToken,omitempty"`
	CreateTimeMs     int64         `json:"create_time_ms,omitempty"`
}

// inboundSessionID coalesces session id variants from getupdates JSON.
func (m weixinMessage) inboundSessionID() string {
	for _, s := range []string{
		m.SessionID.String(), m.SessionIDCamel, m.SessionIDPascal,
		m.MlinkSessionID, m.IlinkSessionID, m.ChatSessionID,
		m.ConversationID, m.ConvID,
	} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

// inboundContextToken coalesces context_token variants from getupdates JSON.
func (m weixinMessage) inboundContextToken() string {
	for _, s := range []string{m.ContextToken, m.ContextTokCamel, m.ContextTokPascal} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

const (
	msgTypeUser   = 1
	msgTypeBot    = 2
	itemTypeText  = 1
	itemTypeImage = 2
	itemTypeVoice = 3
	itemTypeFile  = 4
	itemTypeVideo = 5
)

type sendMessageReq struct {
	Msg      *weixinMessage `json:"msg,omitempty"`
	BaseInfo baseInfo       `json:"base_info"`
}

// getConfigReq matches openclaw api.ts getConfig (ilink/bot/getconfig).
type getConfigReq struct {
	IlinkUserID  string   `json:"ilink_user_id,omitempty"`
	ContextToken string   `json:"context_token,omitempty"`
	BaseInfo     baseInfo `json:"base_info"`
}

type getConfigResp struct {
	Ret          int    `json:"ret"`
	Errcode      int    `json:"errcode"`
	Errmsg       string `json:"errmsg"`
	TypingTicket string `json:"typing_ticket,omitempty"`
	// Session identifiers: some gateways omit session on getupdates but return it from getconfig (needed for sendmessage media).
	SessionID       string `json:"session_id,omitempty"`
	SessionIDCamel  string `json:"sessionId,omitempty"`
	SessionIDPascal string `json:"SessionId,omitempty"`
	MlinkSessionID  string `json:"mlink_session_id,omitempty"`
}

func (r *getConfigResp) coalesceOutboundSessionID() string {
	if r == nil {
		return ""
	}
	for _, s := range []string{r.SessionID, r.SessionIDCamel, r.SessionIDPascal, r.MlinkSessionID} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

// sendTypingReq matches openclaw ilink/bot/sendtyping (before/after sendmessage when typing_ticket is set).
type sendTypingReq struct {
	IlinkUserID  string   `json:"ilink_user_id,omitempty"`
	TypingTicket string   `json:"typing_ticket,omitempty"`
	Status       int      `json:"status,omitempty"`
	BaseInfo     baseInfo `json:"base_info"`
}

const typingStatusTyping = 1
const typingStatusCancel = 2

const sessionExpiredErrcode = -14
