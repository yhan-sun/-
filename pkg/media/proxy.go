package media

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hongguo/pkg/media/cenc"
)

type StreamResolver func(ctx context.Context, seriesID, vid string) (mediaURL string, contentKey []byte, err error)

type cacheEntry struct {
	buffer    []byte
	updatedAt atomic.Int64 // unix milli
}

type Proxy struct {
	listener   net.Listener
	addr       string
	server     *http.Server
	resolver   StreamResolver
	mu         sync.RWMutex
	cache      map[string]*cacheEntry
	cacheSize  int
	maxCache   int
	inflightMu sync.Mutex
	inflight   map[string]chan struct{}
}

func NewProxy(resolver StreamResolver) (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on local port: %w", err)
	}

	p := &Proxy{
		listener: ln,
		addr:     ln.Addr().String(),
		resolver: resolver,
		cache:    make(map[string]*cacheEntry),
		maxCache: 10, // keep up to 10 cached episodes in memory
		inflight: make(map[string]chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", p.handleStream)
	mux.HandleFunc("/local/", p.handleLocalFile)
	p.server = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = p.server.Serve(ln)
	}()

	return p, nil
}

func (p *Proxy) Close() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

func (p *Proxy) StreamURL(seriesID, vid string) string {
	return fmt.Sprintf("http://%s/stream/%s/%s", p.addr, seriesID, vid)
}

// Prewarm starts prefetching and decrypting an episode into in-memory cache in background
func (p *Proxy) Prewarm(seriesID, vid string) {
	if seriesID == "" || vid == "" || p.resolver == nil {
		return
	}
	cacheKey := fmt.Sprintf("%s:%s", seriesID, vid)
	if p.getCached(cacheKey) != nil {
		return
	}

	go func() {
		p.inflightMu.Lock()
		if _, inFlight := p.inflight[cacheKey]; inFlight {
			p.inflightMu.Unlock()
			return
		}
		waitCh := make(chan struct{})
		p.inflight[cacheKey] = waitCh
		p.inflightMu.Unlock()

		defer func() {
			p.inflightMu.Lock()
			delete(p.inflight, cacheKey)
			close(waitCh)
			p.inflightMu.Unlock()
		}()

		bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		mediaURL, contentKey, err := p.resolver(bgCtx, seriesID, vid)
		if err != nil || mediaURL == "" {
			return
		}

		req, err := http.NewRequestWithContext(bgCtx, "GET", mediaURL, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Referer", "https://hongguoduanju.com/")

		client := &http.Client{Timeout: 40 * time.Second}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return
		}
		defer resp.Body.Close()

		rawBytes, err := io.ReadAll(resp.Body)
		if err != nil || len(rawBytes) == 0 {
			return
		}

		var buf []byte
		if len(contentKey) > 0 {
			decrypted, err := cenc.DecryptMp4Buffer(rawBytes, contentKey)
			if err == nil && len(decrypted) > 0 {
				buf = decrypted
			} else {
				buf = rawBytes
			}
		} else {
			buf = rawBytes
		}

		p.setCached(cacheKey, buf)
	}()
}

func (p *Proxy) LocalURL(filePath string) string {
	return fmt.Sprintf("http://%s/local/?path=%s", p.addr, url.QueryEscape(filePath))
}

func (p *Proxy) SetCachedBuffer(seriesID, vid string, data []byte) {
	key := fmt.Sprintf("%s:%s", seriesID, vid)
	p.setCached(key, data)
}

func (p *Proxy) GetCachedBuffer(seriesID, vid string) []byte {
	key := fmt.Sprintf("%s:%s", seriesID, vid)
	return p.getCached(key)
}

func (p *Proxy) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*cacheEntry)
	p.cacheSize = 0
}

func (p *Proxy) CacheSize() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cacheSize
}

func (p *Proxy) handleLocalFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(filePath, "file://") {
		filePath = strings.TrimPrefix(filePath, "file://")
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept, Origin")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")
	w.Header().Set("Accept-Ranges", "bytes")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.ServeFile(w, r, filePath)
}

func parseByteRange(rangeHeader string, totalSize int) (start, end int, hasRange bool, err error) {
	if rangeHeader == "" {
		return 0, totalSize - 1, false, nil
	}
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, false, fmt.Errorf("invalid range header")
	}
	parts := strings.Split(rangeHeader[6:], "-")
	if len(parts) != 2 {
		return 0, 0, false, fmt.Errorf("invalid range format")
	}

	if parts[0] == "" && parts[1] == "" {
		return 0, 0, false, fmt.Errorf("empty range numbers")
	}

	if parts[0] == "" {
		// bytes=-N (last N bytes)
		n, err := strconv.Atoi(parts[1])
		if err != nil || n <= 0 {
			return 0, 0, false, fmt.Errorf("invalid suffix length")
		}
		if n > totalSize {
			n = totalSize
		}
		return totalSize - n, totalSize - 1, true, nil
	}

	s, err := strconv.Atoi(parts[0])
	if err != nil || s < 0 || s >= totalSize {
		return 0, 0, false, fmt.Errorf("invalid range start")
	}

	if parts[1] == "" {
		return s, totalSize - 1, true, nil
	}

	e, err := strconv.Atoi(parts[1])
	if err != nil || e < s {
		return 0, 0, false, fmt.Errorf("invalid range end")
	}
	if e >= totalSize {
		e = totalSize - 1
	}

	return s, e, true, nil
}

func (p *Proxy) getCached(key string) []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if entry, ok := p.cache[key]; ok {
		entry.updatedAt.Store(time.Now().UnixMilli())
		return entry.buffer
	}
	return nil
}

func (p *Proxy) setCached(key string, data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.cache) >= p.maxCache {
		// Evict oldest
		var oldestKey string
		var oldestTime int64
		first := true
		for k, v := range p.cache {
			t := v.updatedAt.Load()
			if first || t < oldestTime {
				oldestKey = k
				oldestTime = t
				first = false
			}
		}
		if oldestKey != "" {
			if old, ok := p.cache[oldestKey]; ok {
				p.cacheSize -= len(old.buffer)
			}
			delete(p.cache, oldestKey)
		}
	}

	entry := &cacheEntry{
		buffer: data,
	}
	entry.updatedAt.Store(time.Now().UnixMilli())
	p.cache[key] = entry
	p.cacheSize += len(data)
}

func (p *Proxy) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept, Origin")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/stream/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "missing seriesID or vid", http.StatusBadRequest)
		return
	}
	seriesID := parts[0]
	vid := parts[1]

	cacheKey := fmt.Sprintf("%s:%s", seriesID, vid)
	buf := p.getCached(cacheKey)

	if buf == nil {
		if p.resolver == nil {
			http.Error(w, "no resolver configured", http.StatusInternalServerError)
			return
		}

		p.inflightMu.Lock()
		waitCh, inFlight := p.inflight[cacheKey]
		if inFlight {
			p.inflightMu.Unlock()
			<-waitCh
			buf = p.getCached(cacheKey)
		} else {
			waitCh = make(chan struct{})
			p.inflight[cacheKey] = waitCh
			p.inflightMu.Unlock()

			defer func() {
				p.inflightMu.Lock()
				delete(p.inflight, cacheKey)
				close(waitCh)
				p.inflightMu.Unlock()
			}()

			// Use decoupled context for upstream downloading so browser probe aborts don't terminate the stream download
			bgCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			mediaURL, contentKey, err := p.resolver(bgCtx, seriesID, vid)
			if err != nil {
				http.Error(w, fmt.Sprintf("resolve failed: %v", err), http.StatusBadGateway)
				return
			}

			// Check if resolver already populated the memory cache
			if cached := p.getCached(cacheKey); cached != nil {
				buf = cached
			} else if strings.HasPrefix(mediaURL, "/") {
				// Local file path
				rawBytes, err := os.ReadFile(mediaURL)
				if err != nil {
					http.Error(w, fmt.Sprintf("read local media failed: %v", err), http.StatusInternalServerError)
					return
				}
				buf = rawBytes
				p.setCached(cacheKey, buf)
			} else {
				// Download media via HTTP
				req, err := http.NewRequestWithContext(bgCtx, "GET", mediaURL, nil)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				req.Header.Set("Referer", "https://hongguoduanju.com/")

				client := &http.Client{
					Transport: &http.Transport{
						Proxy: nil,
					},
					Timeout: 45 * time.Second,
				}
				resp, err := client.Do(req)
				if err != nil {
					http.Error(w, fmt.Sprintf("fetch media failed: %v", err), http.StatusBadGateway)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					http.Error(w, fmt.Sprintf("fetch media HTTP %d", resp.StatusCode), resp.StatusCode)
					return
				}

				rawBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					http.Error(w, fmt.Sprintf("read media failed: %v", err), http.StatusInternalServerError)
					return
				}

				if len(contentKey) > 0 {
					decrypted, err := cenc.DecryptMp4Buffer(rawBytes, contentKey)
					if err != nil {
						http.Error(w, fmt.Sprintf("decrypt failed: %v", err), http.StatusInternalServerError)
						return
					}
					buf = decrypted
				} else {
					buf = rawBytes
				}

				p.setCached(cacheKey, buf)
			}
		}
	}

	totalSize := len(buf)
	rangeHeader := r.Header.Get("Range")
	start, end, hasRange, err := parseByteRange(rangeHeader, totalSize)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
		http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	contentLength := end - start + 1
	w.Header().Set("Content-Length", strconv.Itoa(contentLength))

	if hasRange {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, bytes.NewReader(buf[start:end+1]))
	}
}
