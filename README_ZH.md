# Readform

**重要**：Readform 1.0.0 重磅发布。这一版本使用 Go 语言**完全重写**了整个代码库，使用 chromedp 替换了 selenium，并优化了数据库性能。我们预期通过这一重构，让程序**更加健壮、可靠**。因部分配置字段进行了调整，数据库也进行了优化，如果您在 1.0.0 版本前已经在使用 Readform，建议您升级时**删除 data 目录**，从全新开始使用 Readform。


---------


该程序可以将付费新闻网站的**完整文章内容**发送到您的 [Readwise Reader](https://readwise.io/read) 的 feed 流中，以帮助您获得统一的阅读工作流。将来可能会支持 RSS 输出。

目前支持的网站：
- 端传媒
- 财新
- Financial Times (FT)

未来将支持：
- 华尔街日报
- FT中文网

## 为什么创建 Readform？
市面上有许多基于订阅制的高质量媒体。我非常尊重他们的工作。不过，我相信订户有权以他/她喜欢的不同形式阅读，例如 RSS 阅读器。 **专业读者有自己定制化的阅读工作流。 对文章收费的“专业”媒体应该尊重读者自己的选择。** 由于这些网站没有官方的全文 RSS 支持，我决定自己制作一个。

目前，我使用 Readwise Reader 作为我的 RSS 阅读器，所以我进行了 Readwise Reader 输出集成。 将来也可能支持 RSS 输出。

该项目的最终目标是推动这些网站为其订户提供官方全文 RSS。在此之前，让我们使用这个程序吧！

## 这个程序是如何工作的？
该程序不断使用网站的 RSS 源获取最新文章。当有新文章时，它会模拟浏览器并使用您的凭据登录以获取完整的 HTML 内容。 懒加载的图片会被妥善处理，不用担心图片丢失。 该程序将使用官方 Reader API 将文章 URL 及其 HTML 内容发送到 Readwise Reader，因此您可以在您的 Reader 的 feed 部分中看到它们。

## 财新周刊模式
财新现在支持两种内容模式：

- `latest`（默认）：保留历史 RSS 行为（`https://rsshub.app/caixin/latest`）。
- `weekly_only`：首次启动时将最新一期设为基线，一次性抓取上一期完整周刊，随后持续抓取基线之后的新周刊。

启用 `weekly_only` 后，会在本地数据库跟踪每一期的完整度；某期中尚未成功发送到 Readwise 的文章会持续重试，直到该期全部完成。

为增强登录稳定性，启动时会执行一次财新登录预检（用户名/密码/登录按钮等选择器检查）。若预检因页面变更失败，会记录 warning，并在 `data/diagnostics/` 目录输出诊断截图，同时周刊抓取仍会继续。


## 快速开始
Readform 不是基于云的服务，您需要在自己的机器上运行它。这将为您提供最高程度的安全性，因为使用此程序需要您的网站的用户名和密码。您可以在长期开机的本地设备（PC、Mac、NAS、Raspberry Pi 等）上安装 Readform，或将其部署在 VPS 上。

推荐使用 Docker 运行 Readform。 如果您的计算机上没有 Docker，您可以[在此处下载](https://docs.docker.com/get-docker/)。

请注意，您需要确保程序所处的网络环境可以顺畅访问您希望订阅的网站。

1. 在终端中使用以下命令以在 Docker 中运行此程序。运行后，您将看到一个 docker container ID 的输出：
     ```
     mkdir -p data && \
     docker pull fr0der1c/readform:latest && \
     docker ps -q --filter "name=^readform$" | xargs -r docker stop && \
     docker run --rm --name readform -d -p 5000:5000 -v ./data:/var/app/data fr0der1c/readform:latest
     ```
2. 访问 http://你的设备IP:5000 （如果是在当前电脑上运行，则访问 http://127.0.0.1:5000）并在网页中配置 Readform。
   ![Readform screenshot](./screenshot.png)
3. 一切就绪。新文章将出现在您的 Reader 的 feed 流中。如果它并没有如期工作，您可以使用 `docker logs readform` 命令检查日志，因为本项目仍处于萌芽阶段，并且可能包含 bug。 如果您看到任何异常日志/崩溃/文章不全的情况，请随时通过 GitHub issue 反馈！

## 在 macOS 上构建与校验（仅 amd64）
Readform 的 Docker 镜像目标平台为 `linux/amd64`。在 Apple Silicon 上通过 Docker 模拟运行属于正常行为。

1. 先从已提交的 `HEAD` 回退依赖清单：
     ```
     git restore --source=HEAD -- go.mod go.sum
     ```
2. 在受控的 Go 容器中重新同步并校验依赖：
     ```
     docker run --rm \
       --platform linux/amd64 \
       --user "$(id -u):$(id -g)" \
       -e GOPROXY="https://proxy.golang.org,direct" \
       -e GOSUMDB="sum.golang.org" \
       -e GOMODCACHE="/tmp/gomodcache" \
       -e GOCACHE="/tmp/gocache" \
       -v "$PWD":/src \
       -w /src \
       golang:1.21-bullseye \
       sh -c 'mkdir -p "$GOMODCACHE" "$GOCACHE" && command -v go && go version && go mod tidy && go mod download && go mod verify'
     ```
3. 显式按 amd64 构建镜像：
     ```
     docker build --platform linux/amd64 --no-cache --progress=plain -t readform:weekly-test .
     ```
4. 或使用内置校验脚本：
     ```
     ./scripts/verify.sh
     ```

默认 `go test ./...` 会跳过需要浏览器账号登录的集成测试。若需要运行集成测试，请显式指定：

```
go test -tags integration ./...
```

## FAQ
### 使用此程序需要订阅吗？
这个项目是完全免费和开源的。 但是，**您需要成为网站的订户才能获得完整的文章内容**。 我们不直接提供帐户或完整的文章内容，因为媒体网站和作者都需要获得经济支持以继续前进。

### 这个项目可以绕过付费墙吗？
不能。这个项目的目的是为了改善专业阅读者的工作流程，而不是打破付费墙。您需要使用自己的用户名、密码登录网站。

### 交出我的密码安全吗？
是的。该程序在您的计算机上运行，您的密码将始终保留在本地。我们没有服务器，也不收集用户名和密码。该程序仅对您订阅的站点进行必要的网络请求，并且您的密码将仅在这些站点上使用。

### 如果 Readform 当前不支持我想要的网站怎么办？
我并无计划支持所有付费网站，因为这是一个旨在改善我个人的阅读工作流程的项目。 但是，该项目提供了易于使用的接口，因此您可以轻松添加网站 `Agent`。 欢迎提交 Pull Request！如果您没有代码能力，也可以在 Issue 中提出请求，或许我们（或者社区中的其他成员）会进行跟进。

## 免责声明
使用此代码，即视为您同意以下声明：

本项目**仅供个人使用**，旨在提供更好的阅读体验。 它仅代表用户自动执行操作，**不会以任何方式打破网站所有者设置的任何限制**。 您应自行承担使用本项目的风险。
