# Readform
[Chinese version](./README_ZH.md)

**Important**: Readform 1.0.0 is now released with significant updates. This version has **completely rewritten** the entire codebase in Go language, replaced selenium with chromedp, and optimized database performance. We anticipate that this refactoring will make the program **more robust and reliable**. As some configuration fields have been adjusted and the database has been optimized, if you have been using Readform before version 1.0.0, we suggest you **delete the data directory** when upgrading and start using Readform from scratch.


---------
This program delivers the **full article content** from paywalled news websites directly to your [Readwise Reader](https://readwise.io/read) feed, assisting in creating a unified reading workflow. Support for RSS feed output may be added in the future.

Currently supported websites:
- The initium (端传媒)
- Caixin (财新)
- Financial Times (FT)

Will be supported in the future:
- WSJ（Wall street journal)
- FTChinese（FT中文网）

## Why I built Readform
There's an abundance of high-quality, subscription-based media available on the market, and I have great respect for their work. However, I firmly believe that subscribers should have the freedom to consume content in the format they prefer, such as via an RSS reader. Professional readers often have their own customized reading workflows, and premium media that charge for their content should respect this choice.

Since these websites do not officially support full-article RSS feeds, I took it upon myself to create my own solution. At present, I use Readwise Reader as my RSS reader and have integrated it accordingly. However, I may also add support for RSS output in the future.

The ultimate aim of this project is to encourage these websites to offer official full-content RSS feeds for their subscribers. Until that happens, let's make the most of this program!

## How it works
The program continuously retrieves the latest articles using the website's official RSS feed (or 3rd party RSS feed link, if no official RSS is provided). When new articles are detected, it emulates a browser and logs in with your credentials to access the full HTML content. Any lazy-loading images will be appropriately managed, ensuring no images are missing. The program will then forward the article URL and its HTML content to Readwise Reader via the official Reader API, making them available in your feed section.

## Caixin weekly mode
Caixin now supports two content modes:

- `latest` (default): legacy RSS-based behavior (`https://rsshub.app/caixin/latest`).
- `weekly_only`: first startup sets the current issue as baseline, ingests the previous complete issue once, then keeps ingesting newer weekly issues.

When `weekly_only` is enabled, issue completeness is tracked in local database and unsaved articles are retried until all links in an issue are saved to Readwise.

For login safety, startup includes a Caixin login preflight check (username/password/button selectors). If this check fails due page changes, a warning is logged, a diagnostic screenshot is written under `data/diagnostics/`, and weekly discovery continues.


## Quick start
Readform is not a cloud-based service; rather, it operates on your own device. This approach ensures maximum security, as your username and password are required to use the program. You can install Readform on a local device such as a PC, Mac, NAS, Raspberry Pi, etc., or you can deploy it on a Virtual Private Server (VPS).

Running in Docker is the recommended way to use Readform. If you don't have Docker on your computer, you can [download it here](https://docs.docker.com/get-docker/).

1. To run this program in Docker, you can use the following command in your terminal:
    ```commandline
     mkdir -p data && \
     docker pull fr0der1c/readform:latest && \
     docker ps -q --filter "name=^readform$" | xargs -r docker stop && \
     docker run --rm --name readform -d -p 5000:5000 -v ./data:/var/app/data fr0der1c/readform:latest
    ```
2. Visit http://your-device-ip:5000 (if you're running on current computer, visit http://127.0.0.1:5000) and configure Readform via web UI.
   ![Readform screenshot](./screenshot.png)
3. You're all set. New articles will now appear in your Reader feed section. If you encounter any issues, you can check the logs using the `docker logs readform` command. As this is a relatively new project, it may contain some bugs. If you notice any abnormal logs, crashes, or partial content, please don't hesitate to submit an issue!

## Build and verify on macOS (amd64-only)
Readform Docker image targets `linux/amd64`. On Apple Silicon this runs through Docker emulation, which is expected.

1. Rebaseline module manifests from committed `HEAD`:
    ```commandline
    git restore --source=HEAD -- go.mod go.sum
    ```
2. Re-sync and verify modules in a controlled Go container:
    ```commandline
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
3. Build image explicitly for amd64:
    ```commandline
    docker build --platform linux/amd64 --no-cache --progress=plain -t readform:weekly-test .
    ```
4. Or run the built-in verification script:
    ```commandline
    ./scripts/verify.sh
    ```

Default `go test ./...` skips browser credential integration tests. Run integration tests explicitly with:

```commandline
go test -tags integration ./...
```

## FAQ
### Is a subscription required for using this program?
This project is entirely free and open source. However, it's important to note that **you must be a subscriber to a website to access its full article content**. We do not directly provide accounts or full article content, as it's crucial to financially support the press and authors to ensure their continued operations.

### Does this project bypasses paywall?
No, this project is intended to enhance the workflow for professional readers, not to bypass paywalls. You are required to use your own credentials to log into websites.

### Is it safe to hand over my password?
Yes, the program operates on your computer and your password will always remain local. We do not have servers and we do not collect usernames and passwords. The program only makes necessary network requests to the sites you are subscribed to, and your password will only be used on these sites.

### What if the website I wanted is not supported?
While I don't intend to support all paywalled websites as this project is primarily designed to enhance my own reading workflow, the project does offer user-friendly interfaces. This makes it easy for you to add a website `Agent` as per your needs. Pull requests are always welcome!

## Disclaimer
By using this code, you are deemed to agree with the following statements:

This project is intended **solely for personal use** with the aim of enhancing the reading experience. It merely automates user actions on the user's behalf and **does not in any way breach any restrictions set by the website's owner**. Use at your own risk.
