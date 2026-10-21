package httpapi

import (
	"ainfra/internal/app"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := app.RandomID()
	w.Header().Set("X-Request-ID", requestID)
	if a.logger == nil {
		a.serve(w, r)
		return
	}
	started := time.Now()
	response := &statusWriter{ResponseWriter: w}
	defer func() {
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		method := r.Method
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		default:
			method = "OTHER"
		}
		// Deliberately exclude URL, headers, body and principal credentials.
		a.logger.InfoContext(r.Context(), "http_request", "request_id", requestID, "method", method, "status", status, "duration_ms", float64(time.Since(started).Microseconds())/1000)
	}()
	a.serve(response, r)
}
