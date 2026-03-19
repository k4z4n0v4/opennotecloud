package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateChallenge(account string) (string, int64, error) {
	_, _ = db.Exec(`DELETE FROM login_challenges WHERE account = ? AND datetime(created_at, '+5 minutes') < datetime('now')`, account)
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", 0, err
	}
	const chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	code := make([]byte, 8)
	for i := range code {
		code[i] = chars[int(buf[i])%len(chars)]
	}
	ts := time.Now().UnixMilli()
	_, err := db.Exec(`INSERT INTO login_challenges (account, timestamp, random_code) VALUES (?, ?, ?)`, account, ts, string(code))
	if err != nil {
		return "", 0, err
	}
	return string(code), ts, nil
}

func consumeChallenge(account string, timestamp int64) (string, error) {
	var code string
	if err := db.QueryRow(`SELECT random_code FROM login_challenges WHERE account = ? AND timestamp = ?`, account, timestamp).Scan(&code); err != nil {
		return "", err
	}
	_, _ = db.Exec(`DELETE FROM login_challenges WHERE account = ? AND timestamp = ?`, account, timestamp)
	return code, nil
}

func verifyEquipmentPassword(dbHash, randomCode, submitted string) bool {
	return sha256Hex(dbHash+randomCode) == submitted
}

func sha256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func createJWTToken(cfg *Config, userID int64, equipmentNo string) (string, error) {
	now := time.Now().Unix()
	key := fmt.Sprintf("%d_%d_%d_%s", userID, now, now, equipmentNo)
	claims := jwt.MapClaims{
		"userId": strconv.FormatInt(userID, 10), "createTime": now,
		"equipmentNo": equipmentNo, "key": key,
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	_, err = db.Exec(`INSERT OR REPLACE INTO auth_tokens (key, token, user_id, equipment_no, expires_at) VALUES (?, ?, ?, ?, ?)`,
		key, tokenStr, userID, equipmentNo, expiresAt)
	if err != nil {
		return "", fmt.Errorf("store token: %w", err)
	}
	return tokenStr, nil
}

func validateJWTToken(cfg *Config, tokenStr string) (int64, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return 0, "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, "", fmt.Errorf("invalid JWT claims")
	}
	userIDStr, _ := claims["userId"].(string)
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid userId claim")
	}
	equipmentNo, _ := claims["equipmentNo"].(string)
	key, _ := claims["key"].(string)

	var storedToken string
	var expiresAt time.Time
	err = db.QueryRow(`SELECT token, expires_at FROM auth_tokens WHERE key = ?`, key).Scan(&storedToken, &expiresAt)
	if err == sql.ErrNoRows {
		return 0, "", fmt.Errorf("token not found")
	}
	if err != nil {
		return 0, "", err
	}
	if storedToken != tokenStr {
		return 0, "", fmt.Errorf("token mismatch")
	}
	if time.Now().After(expiresAt) {
		_, _ = db.Exec(`DELETE FROM auth_tokens WHERE key = ?`, key)
		return 0, "", fmt.Errorf("token expired")
	}
	return userID, equipmentNo, nil
}

func deleteToken(tokenStr string, cfg *Config) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if key, ok := claims["key"].(string); ok {
			_, _ = db.Exec(`DELETE FROM auth_tokens WHERE key = ?`, key)
		}
	}
}

func generateUploadSignature(secretKey, path string) (string, string, string) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := generateNonce()
	return sha256Hex(path + ts + nonce + secretKey), ts, nonce
}

// verifySignature verifies a signed URL with the given TTL in milliseconds.
func verifySignature(secretKey, path, signature, timestamp, nonce string, ttlMs int64) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Now().UnixMilli()-ts > ttlMs {
		return false
	}
	return sha256Hex(path+timestamp+nonce+secretKey) == signature
}

func verifyUploadSignature(secretKey, path, signature, timestamp, nonce string) bool {
	return verifySignature(secretKey, path, signature, timestamp, nonce, 30*60*1000)
}

func verifyDownloadSignature(secretKey, path, signature, timestamp, nonce string) bool {
	return verifySignature(secretKey, path, signature, timestamp, nonce, 24*60*60*1000)
}

func generateNonce() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func checkAccountLock(userID int64) (bool, error) {
	var lockedUntil sql.NullTime
	if err := db.QueryRow(`SELECT locked_until FROM users WHERE id = ?`, userID).Scan(&lockedUntil); err != nil {
		return false, err
	}
	return lockedUntil.Valid && time.Now().Before(lockedUntil.Time), nil
}

func recordLoginFailure(userID int64) int {
	_, _ = db.Exec(`UPDATE users SET error_count = 0 WHERE id = ? AND last_error_at < datetime('now', '-12 hours')`, userID)
	_, _ = db.Exec(`UPDATE users SET error_count = error_count + 1, last_error_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
	var count int
	_ = db.QueryRow(`SELECT error_count FROM users WHERE id = ?`, userID).Scan(&count)
	if count >= 6 {
		_, _ = db.Exec(`UPDATE users SET locked_until = datetime('now', '+5 minutes') WHERE id = ?`, userID)
	}
	return count
}

func resetLoginErrors(userID int64) {
	_, _ = db.Exec(`UPDATE users SET error_count = 0, last_error_at = NULL, locked_until = NULL WHERE id = ?`, userID)
}

func fileSignPath(path string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(path))
}

func fileUnsignPath(encoded string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
