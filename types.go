package main

// JSON shapes for ilink/bot API (reference: openclaw-weixin src/api/types.ts).

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

type getUpdatesReq struct {
	GetUpdatesBuf string    `json:"get_updates_buf,omitempty"`
	BaseInfo      baseInfo  `json:"base_info"`
}

type getUpdatesResp struct {
	Ret                   int             `json:"ret"`
	Errcode               int             `json:"errcode"`
	Errmsg                string          `json:"errmsg"`
	Msgs                  []weixinMessage `json:"msgs"`
	GetUpdatesBuf         string          `json:"get_updates_buf"`
	LongpollingTimeoutMs  int             `json:"longpolling_timeout_ms"`
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
	MidSize    int       `json:"mid_size,omitempty"`
}

type voiceItem struct {
	Media *cdnMedia `json:"media,omitempty"`
	Text  string    `json:"text,omitempty"`
}

type fileItem struct {
	Media    *cdnMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	Len      string    `json:"len,omitempty"`
}

type videoItem struct {
	Media      *cdnMedia `json:"media,omitempty"`
	ThumbMedia *cdnMedia `json:"thumb_media,omitempty"`
	VideoSize  int       `json:"video_size,omitempty"`
}

type refMessage struct {
	MessageItem *messageItem `json:"message_item,omitempty"`
	Title       string       `json:"title,omitempty"`
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
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	ItemList     []messageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
}

const (
	msgTypeUser = 1
	msgTypeBot  = 2
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

type getUploadURLReq struct {
	Filekey         string   `json:"filekey,omitempty"`
	MediaType       int      `json:"media_type,omitempty"`
	ToUserID        string   `json:"to_user_id,omitempty"`
	Rawsize         int      `json:"rawsize,omitempty"`
	Rawfilemd5      string   `json:"rawfilemd5,omitempty"`
	Filesize        int      `json:"filesize,omitempty"`
	ThumbRawsize    int      `json:"thumb_rawsize,omitempty"`
	ThumbRawfilemd5 string   `json:"thumb_rawfilemd5,omitempty"`
	ThumbFilesize   int      `json:"thumb_filesize,omitempty"`
	NoNeedThumb     bool     `json:"no_need_thumb,omitempty"`
	Aeskey          string   `json:"aeskey,omitempty"`
	BaseInfo        baseInfo `json:"base_info"`
}

type getUploadURLResp struct {
	UploadParam      string `json:"upload_param,omitempty"`
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"`
}

const (
	uploadMediaImage = 1
	uploadMediaVideo = 2
	uploadMediaFile  = 3
)

const sessionExpiredErrcode = -14
