package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var re = regexp.MustCompile(`(?i)<link>(https?://[^<]*torrents-details\.php\?id=([0-9]+)[^<]*)</link>`)

func fixInvalidXML(body string) string {
	// Finds valid entities like &amp;, &lt;, etc., OR a standalone &
	reAmp := regexp.MustCompile(`&[a-zA-Z0-9#]+;|&`)
	return reAmp.ReplaceAllStringFunc(body, func(m string) string {
		if strings.HasSuffix(m, ";") {
			return m
		}
		return "&amp;"
	})
}

func rewriteRSS(body string) string {
	// 1. Fix the invalid XML (unescaped ampersands)
	compliantBody := fixInvalidXML(body)

	// 2. Rewrite the links and append the enclosures
	return re.ReplaceAllStringFunc(compliantBody, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) == 3 {
			id := submatches[2]
			newURL := fmt.Sprintf("https://cliente.amigos-share.club/download.php?id=%s", id)
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
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

		rewritten := rewriteRSS(string(bodyBytes))

		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		if _, err := w.Write([]byte(rewritten)); err != nil {
			log.Printf("Failed to write response: %v", err)
		}
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting RSS rewriter on :%s, targeting %s", port, targetURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited properly")
}
