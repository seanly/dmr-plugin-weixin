# 微信发送文件/图片/视频实现方案

基于 `@tencent-weixin/openclaw-weixin` 包的源码分析

## 一、整体架构

### 核心流程
```
文件路径 → 判断MIME类型 → 上传到CDN → 发送消息
```

### 关键模块
1. **send-media.ts** - 媒体发送入口，根据MIME类型路由
2. **upload.ts** - 文件上传到微信CDN
3. **cdn-upload.ts** - CDN上传底层实现（AES加密）
4. **send.ts** - 消息发送API调用
5. **api.ts** - HTTP API封装

## 二、详细实现流程

### 1. 发送媒体文件入口 (send-media.ts)

**主函数**: `sendWeixinMediaFile()`

**路由逻辑**:
```typescript
根据文件MIME类型分发：
- video/*  → uploadVideoToWeixin + sendVideoMessageWeixin
- image/*  → uploadFileToWeixin + sendImageMessageWeixin
- 其他     → uploadFileAttachmentToWeixin + sendFileMessageWeixin
```

**参数**:
- `filePath`: 本地文件路径
- `to`: 接收用户ID
- `text`: 附带文本消息
- `opts`: API配置（baseUrl, token, contextToken）
- `cdnBaseUrl`: CDN基础URL

### 2. 文件上传流程 (upload.ts)

**核心函数**: `uploadMediaToCdn()`

**步骤**:

```typescript
1. 读取文件内容 (fs.readFile)
2. 计算文件MD5哈希
3. 生成随机filekey (16字节hex)
4. 生成随机AES key (16字节)
5. 计算AES-128-ECB加密后的文件大小
6. 调用 getUploadUrl API 获取上传参数
7. 调用 uploadBufferToCdn 上传加密文件
8. 返回 UploadedFileInfo
```

**返回数据结构** (`UploadedFileInfo`):
```typescript
{
  filekey: string;                      // 文件唯一标识
  downloadEncryptedQueryParam: string;  // CDN下载加密参数
  aeskey: string;                       // AES密钥(hex编码)
  fileSize: number;                     // 明文文件大小
  fileSizeCiphertext: number;           // 密文文件大小
}
```

**三个具体实现**:
- `uploadFileToWeixin()` - 图片上传 (media_type=1)
- `uploadVideoToWeixin()` - 视频上传 (media_type=2)
- `uploadFileAttachmentToWeixin()` - 文件附件上传 (media_type=3)

### 3. CDN上传实现 (cdn-upload.ts)

**核心函数**: `uploadBufferToCdn()`

**加密方式**: AES-128-ECB + PKCS7 padding

**步骤**:
```typescript
1. 使用AES-128-ECB加密文件内容
2. 构建CDN上传URL (buildCdnUploadUrl)
3. POST加密数据到CDN
   - Content-Type: application/octet-stream
   - Body: 加密后的二进制数据
4. 从响应头获取 x-encrypted-param (下载参数)
5. 支持重试机制 (最多3次)
   - 4xx错误: 立即失败
   - 5xx错误: 重试
```

**关键参数**:
- `buf`: 明文文件内容
- `uploadParam`: 从getUploadUrl获取的上传参数
- `filekey`: 文件唯一标识
- `cdnBaseUrl`: CDN基础URL
- `aeskey`: AES加密密钥

### 4. 消息发送实现 (send.ts)

**三个发送函数**:

#### 4.1 发送图片消息
```typescript
sendImageMessageWeixin({
  to: string,              // 接收用户ID
  text: string,            // 附带文本
  uploaded: UploadedFileInfo,
  opts: { baseUrl, token, contextToken }
})
```

**MessageItem结构**:
```typescript
{
  type: MessageItemType.IMAGE,  // 2
  image_item: {
    media: {
      encrypt_query_param: string,  // CDN下载参数
      aes_key: string,              // base64编码的AES密钥
      encrypt_type: 1               // 加密类型
    },
    mid_size: number                // 密文文件大小
  }
}
```

#### 4.2 发送视频消息
```typescript
sendVideoMessageWeixin({
  to: string,
  text: string,
  uploaded: UploadedFileInfo,
  opts: { baseUrl, token, contextToken }
})
```

**MessageItem结构**:
```typescript
{
  type: MessageItemType.VIDEO,  // 5
  video_item: {
    media: {
      encrypt_query_param: string,
      aes_key: string,
      encrypt_type: 1
    },
    video_size: number  // 密文文件大小
  }
}
```

#### 4.3 发送文件消息
```typescript
sendFileMessageWeixin({
  to: string,
  text: string,
  fileName: string,
  uploaded: UploadedFileInfo,
  opts: { baseUrl, token, contextToken }
})
```

**MessageItem结构**:
```typescript
{
  type: MessageItemType.FILE,  // 4
  file_item: {
    media: {
      encrypt_query_param: string,
      aes_key: string,
      encrypt_type: 1
    },
    file_name: string,
    len: string  // 明文文件大小(字符串)
  }
}
```

### 5. API调用 (api.ts)

#### 5.1 获取上传URL
**端点**: `POST /ilink/bot/getuploadurl`

**请求参数**:
```typescript
{
  filekey: string,        // 随机生成的文件标识
  media_type: number,     // 1=图片, 2=视频, 3=文件, 4=语音
  to_user_id: string,     // 接收用户ID
  rawsize: number,        // 明文文件大小
  rawfilemd5: string,     // 明文文件MD5
  filesize: number,       // 密文文件大小
  aeskey: string,         // AES密钥(hex编码)
  no_need_thumb: boolean, // 不需要缩略图
  base_info: {
    channel_version: string
  }
}
```

**响应**:
```typescript
{
  upload_param: string,        // 上传加密参数
  thumb_upload_param?: string  // 缩略图上传参数(可选)
}
```

#### 5.2 发送消息
**端点**: `POST /ilink/bot/sendmessage`

**请求参数**:
```typescript
{
  msg: {
    from_user_id: string,
    to_user_id: string,
    client_id: string,          // 客户端生成的消息ID
    message_type: 2,            // BOT消息
    message_state: 2,           // FINISH状态
    item_list: MessageItem[],   // 消息项列表
    context_token: string       // 会话上下文token(必需)
  },
  base_info: {
    channel_version: string
  }
}
```

**重要请求头**:
```typescript
{
  "Content-Type": "application/json",
  "AuthorizationType": "ilink_bot_token",
  "Authorization": "Bearer {token}",
  "X-WECHAT-UIN": "{random_base64}",  // 随机生成
  "SKRouteTag": "{route_tag}"         // 可选
}
```

## 三、关键技术点

### 1. 文件加密
- **算法**: AES-128-ECB
- **填充**: PKCS7 padding
- **密钥**: 随机生成16字节
- **密钥编码**:
  - API传输: hex编码
  - 消息发送: base64编码

### 2. MIME类型识别
使用 `getMimeFromFilename()` 根据文件扩展名判断类型：
- `.jpg, .jpeg, .png, .gif, .webp` → image/*
- `.mp4, .mov, .avi, .mkv` → video/*
- 其他 → 文件附件

### 3. 消息发送策略
- 如果有文本 + 媒体: 分两条消息发送
  1. 先发送文本消息 (TEXT item)
  2. 再发送媒体消息 (IMAGE/VIDEO/FILE item)
- 每条消息的 `item_list` 只包含一个 item

### 4. contextToken 机制
- **必需参数**: 所有消息发送都必须携带 contextToken
- **作用**: 关联会话上下文，确保消息在正确的对话中
- **获取**: 从接收到的消息中提取

### 5. 错误处理与重试
- CDN上传支持最多3次重试
- 4xx客户端错误: 立即失败
- 5xx服务器错误: 自动重试
- 超时时间:
  - 普通API: 15秒
  - 长轮询: 35秒
  - 配置API: 10秒

## 四、完整调用示例

### 示例1: 发送图片

```typescript
import { sendWeixinMediaFile } from './messaging/send-media.js';

// 发送本地图片
const result = await sendWeixinMediaFile({
  filePath: '/path/to/image.jpg',
  to: 'user_id_123',
  text: '这是一张图片',
  opts: {
    baseUrl: 'https://api.weixin.qq.com',
    token: 'your_bot_token',
    contextToken: 'context_token_from_inbound_message'
  },
  cdnBaseUrl: 'https://cdn.weixin.qq.com'
});

console.log('消息ID:', result.messageId);
```

### 示例2: 发送视频

```typescript
// 发送本地视频
const result = await sendWeixinMediaFile({
  filePath: '/path/to/video.mp4',
  to: 'user_id_123',
  text: '这是一个视频',
  opts: {
    baseUrl: 'https://api.weixin.qq.com',
    token: 'your_bot_token',
    contextToken: 'context_token_from_inbound_message'
  },
  cdnBaseUrl: 'https://cdn.weixin.qq.com'
});
```

### 示例3: 发送文件附件

```typescript
// 发送PDF文件
const result = await sendWeixinMediaFile({
  filePath: '/path/to/document.pdf',
  to: 'user_id_123',
  text: '这是一个PDF文档',
  opts: {
    baseUrl: 'https://api.weixin.qq.com',
    token: 'your_bot_token',
    contextToken: 'context_token_from_inbound_message'
  },
  cdnBaseUrl: 'https://cdn.weixin.qq.com'
});
```

### 示例4: 下载远程图片并发送

```typescript
import { downloadRemoteImageToTemp } from './cdn/upload.js';

// 1. 下载远程图片到临时目录
const localPath = await downloadRemoteImageToTemp(
  'https://example.com/image.jpg',
  '/tmp/openclaw/weixin/media/outbound-temp'
);

// 2. 发送本地图片
const result = await sendWeixinMediaFile({
  filePath: localPath,
  to: 'user_id_123',
  text: '这是一张远程图片',
  opts: {
    baseUrl: 'https://api.weixin.qq.com',
    token: 'your_bot_token',
    contextToken: 'context_token_from_inbound_message'
  },
  cdnBaseUrl: 'https://cdn.weixin.qq.com'
});
```

## 五、数据流图

```
┌─────────────┐
│  本地文件    │
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│ getMimeFromFilename │ 判断MIME类型
└──────┬──────────────┘
       │
       ├─── video/* ──┐
       ├─── image/* ──┤
       └─── other ────┤
                      │
       ┌──────────────┘
       ▼
┌─────────────────────┐
│ uploadMediaToCdn    │
│  1. 读取文件         │
│  2. 计算MD5         │
│  3. 生成filekey     │
│  4. 生成aeskey      │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ getUploadUrl API    │ 获取上传参数
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ uploadBufferToCdn   │
│  1. AES-ECB加密     │
│  2. POST到CDN       │
│  3. 获取下载参数     │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ UploadedFileInfo    │
│  - filekey          │
│  - downloadParam    │
│  - aeskey           │
│  - fileSize         │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ sendXxxMessageWeixin│
│  1. 构建MessageItem │
│  2. 发送文本(可选)   │
│  3. 发送媒体         │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│ sendMessage API     │ 发送到微信服务器
└─────────────────────┘
```

## 六、关键配置

### 1. 媒体类型枚举
```typescript
UploadMediaType = {
  IMAGE: 1,
  VIDEO: 2,
  FILE: 3,
  VOICE: 4
}

MessageItemType = {
  TEXT: 1,
  IMAGE: 2,
  VOICE: 3,
  FILE: 4,
  VIDEO: 5
}
```

### 2. 消息状态
```typescript
MessageType = {
  NONE: 0,
  USER: 1,  // 用户消息
  BOT: 2    // 机器人消息
}

MessageState = {
  NEW: 0,
  GENERATING: 1,
  FINISH: 2
}
```

### 3. 超时配置
```typescript
DEFAULT_LONG_POLL_TIMEOUT_MS = 35000;  // 长轮询
DEFAULT_API_TIMEOUT_MS = 15000;        // 普通API
DEFAULT_CONFIG_TIMEOUT_MS = 10000;     // 配置API
UPLOAD_MAX_RETRIES = 3;                // CDN上传重试次数
```

## 七、注意事项

### 1. 安全性
- ✅ 所有文件上传前都会进行AES-128-ECB加密
- ✅ 使用随机生成的AES密钥
- ✅ 密钥通过安全通道传输（API请求）
- ✅ CDN下载参数加密

### 2. 性能优化
- 文件大小计算包含PKCS7填充
- 支持CDN上传失败重试
- 使用AbortController实现请求超时控制

### 3. 必需参数
- ⚠️ **contextToken 必需**: 所有消息发送都必须携带，否则会话关联失败
- ⚠️ **token 必需**: API认证令牌
- ⚠️ **to_user_id 必需**: 接收用户ID

### 4. 文件路径处理
- 支持绝对路径
- 支持相对路径（相对于cwd）
- 支持 file:// 协议
- 支持远程URL下载（http/https）

### 5. 错误处理
```typescript
// CDN上传错误
- 4xx: 客户端错误，立即失败
- 5xx: 服务器错误，自动重试（最多3次）
- 超时: 抛出AbortError

// API调用错误
- HTTP错误: 抛出包含状态码和响应体的Error
- 网络错误: 抛出原始错误
- 超时: 抛出AbortError
```

## 八、与现有代码的集成建议

基于你当前项目的 `dmr-plugin-weixin` 实现，建议：

### 1. 扩展现有的 weixinSendText 工具
在现有的文本发送基础上，添加媒体发送能力：

```typescript
// 新增工具: weixinSendMedia
{
  name: 'weixinSendMedia',
  description: '发送图片/视频/文件到微信',
  parameters: {
    to: 'string',
    filePath: 'string',
    text?: 'string',
    mediaType?: 'auto' | 'image' | 'video' | 'file'
  }
}
```

### 2. 复用现有的认证和会话管理
- 使用现有的 token 获取机制
- 复用 contextToken 管理逻辑
- 保持与现有 API 调用的一致性

### 3. 添加文件类型检测
```typescript
import { getMimeFromFilename } from '@tencent-weixin/openclaw-weixin/src/media/mime.js';
```

### 4. 集成CDN上传流程
```typescript
import { uploadFileToWeixin, uploadVideoToWeixin, uploadFileAttachmentToWeixin }
  from '@tencent-weixin/openclaw-weixin/src/cdn/upload.js';
import { sendImageMessageWeixin, sendVideoMessageWeixin, sendFileMessageWeixin }
  from '@tencent-weixin/openclaw-weixin/src/messaging/send.js';
```

---

**总结**: 这套实现方案的核心是"上传-加密-发送"三步走，通过AES加密保证文件安全，通过CDN分发提高传输效率，通过统一的消息格式实现多媒体类型支持。
