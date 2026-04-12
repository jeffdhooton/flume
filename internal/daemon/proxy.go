package daemon

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/jeffdhooton/flume/internal/store"
)

// newProxy creates the reverse proxy with request/response capture.
func (d *Daemon) newProxy() http.Handler {
	target, _ := url.Parse("http://" + d.config.TargetAddr)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = d.proxyErrorHandler

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		id := newULID()

		// Capture request body.
		var reqBody []byte
		var reqTruncated bool
		var reqOrigSize int
		if r.Body != nil && r.ContentLength != 0 {
			reqBody, reqTruncated, reqOrigSize = readLimited(r.Body, store.MaxBodySize)
			r.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		// Wrap the response writer to capture status and body.
		cw := &captureWriter{
			ResponseWriter: w,
			statusCode:     200,
		}

		proxy.ServeHTTP(cw, r)

		duration := time.Since(startedAt)

		// Build the captured request record.
		var respTruncated bool
		var respOrigSize int
		respBody := cw.body.Bytes()
		if len(respBody) > store.MaxBodySize {
			respOrigSize = len(respBody)
			respBody = respBody[:store.MaxBodySize]
			respTruncated = true
		}

		captured := &store.Request{
			ID:                    id,
			Method:                r.Method,
			URL:                   r.URL.String(),
			Path:                  r.URL.Path,
			RequestHeaders:        r.Header,
			RequestBody:           reqBody,
			RequestBodyTruncated:  reqTruncated,
			RequestBodyOrigSize:   reqOrigSize,
			StatusCode:            cw.statusCode,
			ResponseHeaders:       cw.Header().Clone(),
			ResponseBody:          respBody,
			ResponseBodyTruncated: respTruncated,
			ResponseBodyOrigSize:  respOrigSize,
			StartedAt:             startedAt,
			Duration:              duration,
		}

		if d.store != nil {
			if err := d.store.Put(captured); err != nil {
				fmt.Fprintf(logWriter, "flumed: store put: %v\n", err)
			}
		}
	})
}

func (d *Daemon) proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, "flume proxy error: %v", err)
}

// readLimited reads up to max bytes from r, returning the data and whether it was truncated.
func readLimited(r io.Reader, max int) (data []byte, truncated bool, origSize int) {
	buf := make([]byte, max+1)
	n, _ := io.ReadFull(r, buf)
	if n > max {
		return buf[:max], true, n
		// Note: origSize is approximate when truncated since we only read max+1 bytes.
	}
	return buf[:n], false, n
}

// captureWriter wraps http.ResponseWriter to capture the status code and body.
type captureWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	wroteHeader bool
}

func (cw *captureWriter) WriteHeader(code int) {
	if !cw.wroteHeader {
		cw.statusCode = code
		cw.wroteHeader = true
	}
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	cw.body.Write(b)
	return cw.ResponseWriter.Write(b)
}
