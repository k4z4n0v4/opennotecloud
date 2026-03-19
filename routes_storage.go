package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// handleOssUpload handles full file upload via multipart form.
func handleOssUpload(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		signature := q.Get("signature")
		timestamp := q.Get("timestamp")
		nonce := q.Get("nonce")
		encodedPath := q.Get("path")

		// Verify signature.
		if !verifyUploadSignature(cfg.JWTSecret, encodedPath, signature, timestamp, nonce) {
			jsonErrorCode(w, ErrSignatureFailed)
			return
		}

		// Decode path.
		dirPath, err := fileUnsignPath(encodedPath)
		if err != nil {
			jsonError(w, 400, ErrUploadFailed, "invalid path encoding")
			return
		}

		// Get the authenticated user from token header (if present).
		tokenStr := r.Header.Get("x-access-token")
		var userID int64
		var email string
		if tokenStr != "" {
			uid, _, err := validateJWTToken(cfg, tokenStr)
			if err == nil {
				userID = uid
				email, _, _, _ = lookupUserByID(uid)
			}
		}

		// Parse multipart.
		if err := r.ParseMultipartForm(256 << 20); err != nil {
			jsonError(w, 400, ErrUploadFailed, "parse multipart: "+err.Error())
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			jsonError(w, 400, ErrUploadFailed, "missing file field")
			return
		}
		defer file.Close()

		// Determine disk path.
		dirPath = strings.TrimSuffix(dirPath, "/")
		fileName := header.Filename
		dbPath := dirPath + "/" + fileName
		if dirPath == "" || dirPath == "/" {
			dbPath = fileName
		}

		var diskPath string
		if email != "" && userID != 0 {
			diskPath = getDiskPath(email, strings.TrimPrefix(dbPath, "/"), cfg)
		} else {
			diskPath = filepath.Join(cfg.DataDir, strings.TrimPrefix(dbPath, "/"))
		}

		// Ensure directory exists.
		mkdirAll(filepath.Dir(diskPath))

		// Write file.
		dst, err := os.Create(diskPath)
		if err != nil {
			jsonError(w, 500, ErrUploadFailed, "create file: "+err.Error())
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			jsonError(w, 500, ErrUploadFailed, "write file: "+err.Error())
			return
		}

		w.WriteHeader(200)
	}
}

// handleOssUploadPart handles chunked upload.
func handleOssUploadPart(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		signature := q.Get("signature")
		timestamp := q.Get("timestamp")
		nonce := q.Get("nonce")
		encodedPath := q.Get("path")

		if !verifyUploadSignature(cfg.JWTSecret, encodedPath, signature, timestamp, nonce) {
			jsonErrorCode(w, ErrSignatureFailed)
			return
		}

		dirPath, err := fileUnsignPath(encodedPath)
		if err != nil {
			jsonError(w, 400, ErrUploadFailed, "invalid path encoding")
			return
		}

		if err := r.ParseMultipartForm(256 << 20); err != nil {
			jsonError(w, 400, ErrUploadFailed, "parse multipart: "+err.Error())
			return
		}

		partNumberStr := r.FormValue("partNumber")
		totalChunksStr := r.FormValue("totalChunks")
		uploadID := r.FormValue("uploadId")

		partNumber, _ := strconv.Atoi(partNumberStr)
		totalChunks, _ := strconv.Atoi(totalChunksStr)

		if uploadID == "" {
			uploadID = generateNonce()
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			jsonError(w, 400, ErrUploadFailed, "missing file field")
			return
		}
		defer file.Close()

		// Get user info.
		tokenStr := r.Header.Get("x-access-token")
		var email string
		if tokenStr != "" {
			uid, _, err := validateJWTToken(cfg, tokenStr)
			if err == nil {
				email, _, _, _ = lookupUserByID(uid)
			}
		}

		// Save chunk to temp directory.
		tempDir := filepath.Join(cfg.DataDir, ".chunks", uploadID)
		mkdirAll(tempDir)

		chunkPath := filepath.Join(tempDir, fmt.Sprintf("part_%05d", partNumber))
		data, _ := io.ReadAll(file)
		if err := os.WriteFile(chunkPath, data, 0644); err != nil {
			jsonError(w, 500, ErrUploadFailed, "write chunk: "+err.Error())
			return
		}

		// Compute chunk MD5.
		hash := md5.Sum(data)
		chunkMD5 := hex.EncodeToString(hash[:])

		// Track chunk in DB.
		_, _ = db.Exec(`INSERT OR REPLACE INTO chunk_uploads (upload_id, part_number, total_chunks, chunk_md5, path) VALUES (?, ?, ?, ?, ?)`,
			uploadID, partNumber, totalChunks, chunkMD5, dirPath)

		// Check if all chunks are received.
		var uploadedCount int
		_ = db.QueryRow(`SELECT COUNT(*) FROM chunk_uploads WHERE upload_id = ?`, uploadID).Scan(&uploadedCount)

		if uploadedCount >= totalChunks {
			// Merge chunks.
			dirPath = strings.TrimSuffix(dirPath, "/")
			fileName := header.Filename
			dbPath := dirPath + "/" + fileName
			if dirPath == "" || dirPath == "/" {
				dbPath = fileName
			}

			var diskPath string
			if email != "" {
				diskPath = getDiskPath(email, strings.TrimPrefix(dbPath, "/"), cfg)
			} else {
				diskPath = filepath.Join(cfg.DataDir, strings.TrimPrefix(dbPath, "/"))
			}

			mkdirAll(filepath.Dir(diskPath))
			merged, err := os.Create(diskPath)
			if err == nil {
				for i := 1; i <= totalChunks; i++ {
					cp := filepath.Join(tempDir, fmt.Sprintf("part_%05d", i))
					cd, _ := os.ReadFile(cp)
					merged.Write(cd)
				}
				merged.Close()
			}

			// Cleanup.
			os.RemoveAll(tempDir)
			_, _ = db.Exec(`DELETE FROM chunk_uploads WHERE upload_id = ?`, uploadID)
		}

		jsonSuccess(w, map[string]interface{}{
			"uploadId":    uploadID,
			"partNumber":  partNumber,
			"totalChunks": totalChunks,
			"chunkMd5":    chunkMD5,
			"status":      "SUCCESS",
		})
	}
}

// handleOssDownload serves file bytes with Range header support.
func handleOssDownload(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		encodedPath := q.Get("path")
		signature := q.Get("signature")
		timestamp := q.Get("timestamp")
		nonce := q.Get("nonce")
		pathIDStr := q.Get("pathId")

		if !verifyDownloadSignature(cfg.JWTSecret, encodedPath, signature, timestamp, nonce) {
			http.Error(w, "signature verification failed", 403)
			return
		}

		dbPath, err := fileUnsignPath(encodedPath)
		if err != nil {
			http.Error(w, "invalid path", 400)
			return
		}

		// Determine user email from pathId (userId).
		var email string
		if pathIDStr != "" {
			uid, _ := strconv.ParseInt(pathIDStr, 10, 64)
			if uid != 0 {
				email, _, _, _ = lookupUserByID(uid)
			}
		}

		var diskPath string
		if email != "" {
			diskPath = getDiskPath(email, strings.TrimPrefix(dbPath, "/"), cfg)
		} else {
			diskPath = filepath.Join(cfg.DataDir, strings.TrimPrefix(dbPath, "/"))
		}

		f, err := os.Open(diskPath)
		if err != nil {
			http.Error(w, "file not found", 404)
			return
		}
		defer f.Close()

		stat, _ := f.Stat()
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	}
}
