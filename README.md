# Kwai Premium API Go SDK

快手 Premium 短剧开放平台的 Go SDK，覆盖文档中全部 **6 个业务接口及 2 个存储上传接口**。支持图片上传、视频上传和转码查询、短剧创建、幂等键校验、限流错误信息与 context 取消。Go 1.22+，仅依赖标准库。

[接口与约束说明](docs/api.md) · [上传示例](examples/upload/main.go) · [创建短剧示例](examples/create-drama/main.go)

## 本地接入

SDK 为独立模块，目录为 E:/kwai-premium-api-go。module 预设为 github.com/isaac-cui/kwai-premium-api-go，可通过本地替换接入其他 Go 项目：

```go
require github.com/isaac-cui/kwai-premium-api-go v0.0.0

replace github.com/isaac-cui/kwai-premium-api-go => ../kwai-premium-api-go
```

API Key 由快手对接人员分配。将值放入环境变量 KWAI_PREMIUM_API_KEY。默认生产域名为 https://premium.kwai.com；文档没有测试环境。

```go
client, err := premium.New(premium.Config{
    APIKey: os.Getenv("KWAI_PREMIUM_API_KEY"),
})
if err != nil { return err }
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
defer cancel()
```

调用片段需要导入 context、os、time 和：

```go
premium "github.com/isaac-cui/kwai-premium-api-go"
```

## 图片上传

```go
file, err := os.Open("cover.jpg")
if err != nil { return err }
defer file.Close()

response, err := client.Images.Upload(ctx, file, "jpg")
if err != nil { return err }
imageKey := response.Data.ImageKey
_ = imageKey
```

Upload 自动完成申请 Token → 分片直传 → complete → 图片提交。每个文件申请独立 token，文件流由调用方关闭。也可分别调用 Images.UploadToken、client.NewUploader、Uploader.Fragment/Complete 和 Images.Submit。

## 视频上传和转码

```go
response, err := client.Videos.Upload(ctx, videoReader, "mp4")
if err != nil { return err }
videoID := response.Data.VideoID

_, err = client.Videos.WaitForTranscode(ctx, videoID, premium.PollOptions{})
if err != nil { return err }
// 此时视频转码成功，可使用 videoID 创建短剧。
```

Upload 仅执行一次申请 Token → 存储上传并完成 → 提交视频，不重复触发转码。WaitForTranscode 默认每 10 秒查询一次，总超时 30 分钟；支持自定义 Interval、Timeout，父 context 可提前取消。status=2 成功后立即返回；status=3 返回 *TranscodeError；未知状态或业务错误立即返回。限流时返回 *APIError，包含 RetryAfter，交由调用方处理。

存储服务器地址无 scheme 时默认补 HTTPS。存储请求使用独立 HTTP 客户端，仅通过 upload_token 鉴权。默认逐片最多读取 4 MiB，不将整个视频载入内存。空文件、读失败和上传失败均不会调用 complete 或后续提交接口。

## 创建短剧

```go
request := premium.CreateDramaRequest{
    IdempotencyKey: "your-stable-business-id",
    Title: "我的短剧",
    CoverImageKey: "previously-uploaded-cover",
    HeaderImages: []premium.HeaderImage{
        {ImageKey: "previously-uploaded-header"},
    },
    Episodes: []premium.Episode{
        {EpisodeIndex: 0, VideoID: "transcoded-video-0", Title: "第一集", ModuleFeeType: premium.EpisodeFree},
        {EpisodeIndex: 1, VideoID: "transcoded-video-1", Title: "第二集", ModuleFeeType: premium.EpisodePaid},
    },
}
response, err := client.Dramas.Create(ctx, request)
if err != nil { return err }
dramaID := response.Data.DramaID // int64，不经过 float64
_ = dramaID
```

Create 自动执行 Validate：字段长度、必填信息、0 起始且不重复的剧集序号、剧集数量和免费/付费结构。支持乱序传入，校验不会修改原数组。HeaderImages 必须显式赋值，nil 与 [] 的约定详见接口说明。

同一账号 24 小时内相同 IdempotencyKey 返回首次成功的 dramaId。调用方应持久化 key，并在超时后的人工/业务重试中沿用同一 key。SDK 不自动重放写入请求。创建后审核操作按文档 FAQ 在 Premium 平台完成。

## 错误和诊断

```go
var apiErr *premium.APIError
if errors.As(err, &apiErr) {
    fmt.Println(apiErr.Code, apiErr.Message, apiErr.TraceID)
    if apiErr.IsRateLimited() {
        fmt.Println("retry delay:", apiErr.RetryAfter)
    }
}
```

此片段额外导入 errors 和 fmt。HTTP 200 下 result=0 仍会返回 APIError。该错误保留 HTTPStatus、Result、Code、Message、TraceID、Header、Raw 和 RetryAfter。所有文档错误码均在 errors.go 中提供常量。

业务成功响应 Response[T] 包含强类型 Data、TraceID、HTTPStatus、Header 与原始 JSON Raw。视频 ID 为 string，短剧 ID 为 int64。client.Call(ctx, "/open/v1/...", request) 支持未来接口，Data 保持 json.RawMessage，避免大整数精度损失。

ValidationError 表示本地字段错误，ProtocolError 表示响应格式异常，TransportError 包装网络/读响应错误且默认错误文本不打印可能含 token 的 URL。context 取消和超时可以通过 errors.Is 检测。

## 配置与测试

Config 可设置 BaseURL、HTTPClient、UploadHTTPClient、MaxResponseBytes；默认单次 HTTP 超时 30 秒、最大响应 4 MiB。HTTPClient 与 UploadHTTPClient 分开配置，避免业务鉴权中间件流向存储服务。传入的客户端会被复制并禁用重定向，原对象不变。

```sh
go test -cover ./...
go vet ./...

# 运行前设置 KWAI_PREMIUM_API_KEY，以下命令会实际上传或创建内容。
go run ./examples/upload -kind image -file cover.jpg -type jpg
go run ./examples/upload -kind video -file episode.mp4 -type mp4 -wait
go run ./examples/create-drama ./examples/create-drama/request.example.json
```

创建示例必须先替换 JSON 中的图片 Key、已转码成功的视频 ID 和幂等键。自动化测试仅使用本地模拟 HTTP 服务，不需要真实 API Key；尚未进行生产账号联调。
