package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type SnowflakeID int64

func (s SnowflakeID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(int64(s), 10) + `"`), nil
}

func jsonSuccess(w http.ResponseWriter, extra map[string]interface{}) {
	resp := map[string]interface{}{"success": true, "errorCode": nil, "errorMsg": nil}
	for k, v := range extra {
		resp[k] = v
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(resp)
}

func jsonError(w http.ResponseWriter, httpCode int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpCode)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(map[string]interface{}{"success": false, "errorCode": code, "errorMsg": msg})
}

func jsonErrorCode(w http.ResponseWriter, code string) {
	jsonError(w, 200, code, errMsg(code))
}

func parseJSONBody(r *http.Request) map[string]interface{} {
	if r.Body == nil {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if dec.Decode(&m) != nil {
		return map[string]interface{}{}
	}
	return m
}

func bodyStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func bodyInt(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

func bodyBool(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	}
	return false
}

func datetimeToEpochMillis(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}
