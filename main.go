package main

import (
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

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	})

	// Intercept /download.php to proxy the torrent download
	mux.HandleFunc("/download.php", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		downloadURL := fmt.Sprintf("%s/download.php?id=%s", targetBaseURL, id)
		req, err := http.NewRequestWithContext(r.Context(), "GET", downloadURL, nil)
		if err != nil {
			http.Error(w, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}

		// Copy useful headers from the original request (like User-Agent)
		req.Header.Set("User-Agent", r.Header.Get("User-Agent"))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "Failed to download torrent: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response headers (like Content-Disposition, Content-Type)
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)

		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("Error copying torrent body: %v", err)
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
		log.Printf("Starting RSS rewriter on :%s", port)
		log.Printf("Targeting RSS Base: %s", targetBaseURL)
		if cookie != "" {
			log.Printf("Cookie auth enabled for torrent downloads")
		} else {
			log.Printf("WARNING: No COOKIE set! Downloads may fail if authentication is required.")
		}

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
