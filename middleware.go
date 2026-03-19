package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type contextKey string

const (
	ctxUserID contextKey = "userID"
	ctxToken  contextKey = "token"
)

func authMiddleware(cfg *Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("x-access-token")
		if tokenStr == "" {
			jsonError(w, 401, ErrNotLoggedIn, errMsg(ErrNotLoggedIn))
			return
		}
		userID, _, err := validateJWTToken(cfg, tokenStr)
		if err != nil {
			jsonError(w, 401, ErrNotLoggedIn, errMsg(ErrNotLoggedIn))
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxToken, tokenStr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUserID(r *http.Request) int64 {
	v, _ := r.Context().Value(ctxUserID).(int64)
	return v
}

func getToken(r *http.Request) string {
	v, _ := r.Context().Value(ctxToken).(string)
	return v
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC: %v\n%s", rec, debug.Stack())
				jsonError(w, 500, ErrSystem, errMsg(ErrSystem))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseCapture struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}
func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return rc.ResponseWriter.Write(b)
}

const maxLogBody = 8192

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		path := r.URL.Path

		if strings.HasPrefix(path, "/socket.io/") {
			log.Printf("=> %s %s %s [websocket]", r.Method, path, r.RemoteAddr)
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(path, "/api/oss/") {
			rc := &responseCapture{ResponseWriter: w, statusCode: 200}
			next.ServeHTTP(rc, r)
			log.Printf("=> %s %s %s [%d] (%s) [binary endpoint, body not logged]",
				r.Method, path, r.RemoteAddr, rc.statusCode, time.Since(start))
			return
		}

		var reqBody []byte
		if r.Body != nil {
			reqBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(reqBody))
		}
		rc := &responseCapture{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rc, r)
		log.Printf("=> %s %s %s [%d] (%s)\n   REQ:  %s\n   RESP: %s",
			r.Method, path, r.RemoteAddr, rc.statusCode, time.Since(start),
			truncate(string(reqBody), maxLogBody), truncate(rc.body.String(), maxLogBody))
	})
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", ""))
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
