# 🚀 ASC RSS Rewriter

[![Build and Push Docker Image](https://github.com/williamokano/asc-rss-rewriter/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/williamokano/asc-rss-rewriter/actions/workflows/docker-publish.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/williamokano/asc-rss-rewriter)](https://goreportcard.com/report/github.com/williamokano/asc-rss-rewriter)

A blazing-fast, lightweight HTTP proxy written in Go, specifically designed to intercept, rewrite, and fix invalid BitTorrent RSS feeds on-the-fly.

## ✨ Why does this exist?

Some BitTorrent RSS feeds (like Amigos Share Club) output links pointing to web detail pages instead of direct `.torrent` downloads. Additionally, they often produce **invalid XML** by leaving characters like `&` unescaped (e.g., `&hit=1`), which causes strict RSS readers and automated downloaders to crash.

**ASC RSS Rewriter acts as a middleman:**
1. **Fixes XML Syntax:** Scans the entire feed and smartly escapes invalid standalone ampersands (`&` → `&amp;`) without destroying existing entities, rendering the feed 100% compliant.
2. **Rewrites Links:** Detects item detail links and rewrites them into direct `download.php` links.
3. **Injects Enclosures:** Automatically injects the correct BitTorrent `<enclosure>` tags so your torrent client (qBittorrent, Deluge, etc.) recognizes them immediately.

---

## 🛠️ How to run it

### Option 1: Docker (Recommended 🐳)

This project is automatically built and published as a tiny multi-arch Alpine image on the GitHub Container Registry (GHCR).

```bash
docker run -d \
  -p 8080:8080 \
  -e RSS_URL="https://cliente.amigos-share.club/rss.php?cat=69...&passkey=YOUR_PASSKEY_HERE" \
  -e COOKIE="uid=12345; pass=abcdef..." \
  --name asc-rss-rewriter \
  ghcr.io/williamokano/asc-rss-rewriter:latest
```

*(The image includes a built-in Docker `HEALTHCHECK` mapped to the `/healthz` endpoint for automated orchestration restarts!)*

### Option 2: Build from Source (Go 🐹)

```bash
# Clone the repository
git clone https://github.com/williamokano/asc-rss-rewriter.git
cd asc-rss-rewriter

# Build the binary
go build -o rewriter main.go

# Export your target RSS feed URL
export RSS_URL="https://cliente.amigos-share.club/rss.php?cat=69...&passkey=YOUR_PASSKEY"

# Export your cookie so the proxy can download the .torrent files on behalf of your client
export COOKIE="uid=12345; pass=abcdef..."

# Optional: Change the listening port (defaults to 8080)
export PORT=8080

# Run it!
./rewriter
```

---

## 🎯 Usage

Once the proxy is running, point your Torrent Client's RSS reader to your local proxy instead of the real URL:

```
http://localhost:8080/
```

### Endpoints
* `/` - Proxies and rewrites the RSS feed.
* `/healthz` - Liveness probe, returns `HTTP 200 ok`.

---

## 🏗️ CI/CD Setup

This repository uses GitHub Actions (`.github/workflows/docker-publish.yml`). 
Whenever you push to the `main` branch, it automatically:
- Tests the Go codebase.
- Compiles the application dynamically for both `amd64` and `arm64` CPU architectures.
- Pushes it to `ghcr.io/williamokano/asc-rss-rewriter`.

**No secret configuration required!** It uses GitHub's native `GITHUB_TOKEN` to publish directly to the Packages tab of this repository.
