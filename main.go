package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var re = regexp.MustCompile(`(?i)<link>(https?://[^<]*torrents-details\.php\?id=([0-9]+)[^<]*)</link>`)

// logLevel gates which messages actually get printed; DEBUG is the most
// verbose. Set at startup from LOG_LEVEL and never mutated afterwards, so
// no synchronization is needed even though it's read from handler goroutines.
type logLevel int

const (
	levelDebug logLevel = iota
	levelInfo
	levelWarn
	levelError
)

var currentLogLevel = levelInfo

func parseLogLevel(s string) logLevel {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return levelDebug
	case "WARN", "WARNING":
		return levelWarn
	case "ERROR":
		return levelError
	default:
		return levelInfo
	}
}

func logDebug(format string, args ...interface{}) { logAt(levelDebug, format, args...) }
func logInfo(format string, args ...interface{})  { logAt(levelInfo, format, args...) }
func logWarn(format string, args ...interface{})  { logAt(levelWarn, format, args...) }

func logAt(level logLevel, format string, args ...interface{}) {
	if level < currentLogLevel {
		return
	}
	log.Printf(format, args...)
}

// cappedBuffer retains at most limit bytes written to it; excess writes are
// discarded rather than erroring, so it's safe to plug into an io.Copy that
// must still deliver every byte to the real destination.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		c.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func fixInvalidXML(body string) string {
	reAmp := regexp.MustCompile(`&[a-zA-Z0-9#]+;|&`)
	return reAmp.ReplaceAllStringFunc(body, func(m string) string {
		if strings.HasSuffix(m, ";") {
			return m
		}
		return "&amp;"
	})
}

// rewriteRSS rewrites links to point back to the proxy itself (proxyHost)
func rewriteRSS(body string, proxyHost string) string {
	compliantBody := fixInvalidXML(body)

	return re.ReplaceAllStringFunc(compliantBody, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) == 3 {
			id := submatches[2]
			// Point the download back to our proxy
			newURL := fmt.Sprintf("http://%s/download.php?id=%s", proxyHost, id)
			return fmt.Sprintf("<link>%s</link>\n<enclosure url=\"%s\" type=\"application/x-bittorrent\"/>", newURL, newURL)
		}
		return match
	})
}

func main() {
	targetURL := os.Getenv("RSS_URL")
	if targetURL == "" {
		log.Fatal("RSS_URL environment variable is required")
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		log.Fatalf("Invalid RSS_URL: %v", err)
	}
	targetBaseURL := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	cookie := os.Getenv("COOKIE")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	currentLogLevel = parseLogLevel(os.Getenv("LOG_LEVEL"))

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	})

	// Client used for torrent downloads; logs every hop so redirects to a
	// login page (e.g. an expired/invalid cookie) are visible instead of
	// being silently followed.
	downloadClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			logInfo("[download] redirect: %s -> %s", via[len(via)-1].URL, req.URL)
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	// Intercept /download.php to proxy the torrent download
	mux.HandleFunc("/download.php", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		logInfo("[download] received request: id=%s remote=%s ua=%q", id, r.RemoteAddr, r.Header.Get("User-Agent"))
		if id == "" {
			logWarn("[download] rejected: missing id parameter")
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		downloadURL := fmt.Sprintf("%s/download.php?id=%s", targetBaseURL, id)
		req, err := http.NewRequestWithContext(r.Context(), "GET", downloadURL, nil)
		if err != nil {
			logWarn("[download] id=%s failed to build upstream request: %v", id, err)
			http.Error(w, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}

		// Copy useful headers from the original request (like User-Agent)
		req.Header.Set("User-Agent", r.Header.Get("User-Agent"))

		logInfo("[download] id=%s proxying to upstream: %s (cookie set=%t)", id, downloadURL, cookie != "")
		resp, err := downloadClient.Do(req)
		if err != nil {
			logWarn("[download] id=%s upstream request failed: %v", id, err)
			http.Error(w, "Failed to download torrent: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		logInfo("[download] id=%s upstream response: status=%d final_url=%s content-type=%q content-length=%s",
			id, resp.StatusCode, resp.Request.URL, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"))

		// Copy response headers (like Content-Disposition, Content-Type)
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)

		// At DEBUG, mirror up to 4KB of the response body into the log too
		// (regardless of content type) so a bad response (e.g. an HTML
		// error/throttle page instead of a torrent) is visible without
		// having to reproduce it by hand.
		dest := io.Writer(w)
		var preview *cappedBuffer
		if currentLogLevel <= levelDebug {
			preview = &cappedBuffer{limit: 4096}
			dest = io.MultiWriter(w, preview)
		}

		written, err := io.Copy(dest, resp.Body)
		if err != nil {
			logWarn("[download] id=%s error copying torrent body: %v", id, err)
			return
		}
		logInfo("[download] id=%s returned to client: status=%d bytes=%d", id, resp.StatusCode, written)
		if preview != nil {
			logDebug("[download] id=%s response body preview (first %d bytes): %q", id, preview.buf.Len(), preview.buf.String())
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get(targetURL)
		if err != nil {
			http.Error(w, "Failed to fetch RSS: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Failed to read RSS: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Rewrite using the Host header so it points back to this proxy instance
		rewritten := rewriteRSS(string(bodyBytes), r.Host)

		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		if _, err := w.Write([]byte(rewritten)); err != nil {
			logWarn("Failed to write response: %v", err)
		}
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logInfo("Starting RSS rewriter on :%s", port)
		logInfo("Targeting RSS Base: %s", targetBaseURL)
		if cookie != "" {
			logInfo("Cookie auth enabled for torrent downloads")
		} else {
			logWarn("WARNING: No COOKIE set! Downloads may fail if authentication is required.")
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	logInfo("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	logInfo("Server exited properly")
}
