# Premium API 接口与行为

来源：[premium 短剧开放平台 API 对外接入文档](https://docs.qingque.cn/d/home/eZQDj56DClU9vLT6XGxZfHzSM?identityId=201KbcJskKa#section=h.cx3tp9b10xa0)，核对日期：2026-11-11。

生产域名 https://premium.kwai.com；文档未提供测试环境。业务请求统一为 POST + UTF-8 JSON，通过 Authorization: Bearer API_KEY 鉴权。HTTP 200 不等于业务成功，须检查 result=1。SDK 保留 errorCode/errorMsg/traceId 和 Retry-After。

## 完整接口映射

| Go 方法 | 路径 | 请求类型 | 响应 data |
|---|---|---|---|
| Images.UploadToken | /open/v1/resource/image/token | ImageTokenRequest：imageType | token、uploadEndpoint |
| Images.Submit | /open/v1/resource/image/submit | ImageSubmitRequest：token | imageKey |
| Videos.UploadToken | /open/v1/resource/video/token | VideoTokenRequest：videoType | token、uploadEndpoint |
| Videos.Submit | /open/v1/resource/video/submit | VideoSubmitRequest：token、videoType | videoId（string） |
| Videos.Transcode | /open/v1/resource/video/transcode | VideoTranscodeRequest：videoId | status（1 处理中、2 成功、3 失败） |
| Dramas.Create | /open/v1/drama/create | CreateDramaRequest | dramaId（int64） |
| Uploader.Fragment | /api/upload/fragment | Query：fragment_id、upload_token；Body：原始字节 | HTTP 200 为成功，保留原始响应 |
| Uploader.Complete | /api/upload/complete | Query：fragment_count、upload_token；空 Body | HTTP 200 为成功，保留原始响应 |

图片格式 jpg/jpeg/png/webp；视频格式 mp4/mov，大小写不敏感。视频提交时保留申请 token 使用的格式。存储上传使用独立客户端，不添加 Premium Authorization；uploadEndpoint 无 scheme 时补 HTTPS。默认每片 4 MiB、索引从 0 递增，全部成功后 complete；单片大文件可以手动调用 Fragment，文档未强制要求 4 MiB 上限。

## 创建短剧

| 字段 | Go 类型 | 约束 |
|---|---|---|
| idempotencyKey | string | 必填、最长 128 字符；由调用方生成并持久化 |
| title | string | 必填、最长 100 字符 |
| description | string | 可选、最长 1500 字符 |
| coverImageKey | string | 必填，由 Images.Submit 获取 |
| headerImages | []HeaderImage | 必须显式提供数组，每个元素 imageKey 必填 |
| episodes | []Episode | 1～130 项，同时满足免费/付费规则 |
| episodes[].episodeIndex | int | 从 0 开始，非负且唯一，可乱序传入 |
| episodes[].videoId | string | 必填，使用本账号提交且转码成功的视频 |
| episodes[].title | string | 必填、最长 100 字符 |
| episodes[].description | string | 可选、最长 300 字符 |
| episodes[].moduleFeeType | int | 1 免费、2 付费 |

按照 episodeIndex 排序后，须至少一集免费、至少一集付费，免费只能是开头连续的 N 集。因而实际可创建的有效剧集组合至少为 2 集。Validate 在副本上排序校验，发送时保留原剧集顺序。SDK 不校验视频归属和远端转码状态，这两项由服务端最终确认。

幂等键按账号邮箱隔离，同一账号 24 小时内使用相同 key 重复成功请求会返回首次 dramaId。网络超时后请保留相同 key，避免另建 key 导致重复创建。SDK 不自动生成或替换 key，也不重试写入。

## 默认限流

| 接口 | 每分钟请求数 | 每日请求数 |
|---|---:|---:|
| 图片 Token、图片提交、视频 Token、视频提交（分别计数） | 60 | 15000 |
| 转码查询 | 10000 | 不限 |
| 短剧创建 | 10 | 200 |

限流按账号邮箱 × 接口计算，同一邮箱的 API Key 共享配额。业务上另有每日 100 部短剧创建配额，与每日 200 次接口访问限制相互独立。RATE_LIMIT_EXCEEDED 对应访问限流，OPEN_API_DAILY_QUOTA_EXCEEDED 对应创建数量限制。SDK 读取 Retry-After 秒数，不自动消耗共享配额重试。

## 文档边界

- 文档描述 headerImages 必填，元素 imageKey 在列表非空时必填，没有明确列表最小长度。SDK 要求非 nil，允许显式 []；如业务要求头图，应传入至少一个有效 HeaderImage。
- 文档创建章节同时出现“草稿”和“进入审核流程”的表述，FAQ 明确未提供提交审核与审核状态查询 API。SDK 仅创建短剧；审核须在 Premium 平台操作。
- 直传只按 HTTP 200 判断成功，与 result=1 的 Premium 业务响应契约不同；SDK 接受空或非 JSON 的成功存储响应。
- 当前只依据文档和本地模拟服务完成实现，未对生产账号进行联调。
