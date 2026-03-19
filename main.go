package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "create-user" {
		cmdCreateUser()
		return
	}
	cfg := loadConfig()
	if err := initDB(cfg.DBPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	var secret string
	_ = db.QueryRow(`SELECT value FROM server_settings WHERE key = 'jwt_secret'`).Scan(&secret)
	if secret != "" {
		cfg.JWTSecret = secret
	} else {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			log.Fatalf("Failed to generate JWT secret: %v", err)
		}
		cfg.JWTSecret = hex.EncodeToString(buf)
		_, _ = db.Exec(`INSERT INTO server_settings (key, value) VALUES ('jwt_secret', ?)`, cfg.JWTSecret)
	}
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	mux := http.NewServeMux()
	registerRoutes(mux, cfg)
	handler := recoveryMiddleware(loggingMiddleware(mux))
	log.Printf("OpenNoteCloud listening on %s (base URL: %s)", cfg.Listen, cfg.BaseURL)
	if err := http.ListenAndServe(cfg.Listen, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func cmdCreateUser() {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)
	email := fs.String("email", "", "User email address")
	passHash := fs.String("password-hash", "", "MD5 hex of the user password")
	username := fs.String("username", "", "Display name (optional)")
	dbPath := fs.String("db", "", "Database path (default: from OPENNOTE_DB_PATH)")
	fs.Parse(os.Args[2:])
	if *email == "" || *passHash == "" {
		fmt.Fprintln(os.Stderr, "Usage: opennotecloud create-user --email=<email> --password-hash=<md5hex>")
		os.Exit(1)
	}
	dp := *dbPath
	if dp == "" {
		dp = envOr("OPENNOTE_DB_PATH", "/data/opennotecloud.db")
	}
	if err := initDB(dp); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	if err := createUser(*email, *passHash, *username); err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}
}

func registerRoutes(mux *http.ServeMux, cfg *Config) {
	auth := func(h http.HandlerFunc) http.Handler { return authMiddleware(cfg, h) }

	// System (public)
	mux.HandleFunc("GET /api/file/query/server", handleQueryServer)

	// Auth (public)
	mux.HandleFunc("POST /api/official/user/query/random/code", handleQueryRandomCode)
	mux.HandleFunc("POST /api/official/user/check/exists/server", handleCheckUserExistsServer)
	mux.HandleFunc("POST /api/official/user/account/login/equipment", handleEquipmentLogin(cfg))
	mux.HandleFunc("POST /api/terminal/user/bindEquipment", handleBindEquipment)
	mux.HandleFunc("POST /api/terminal/equipment/unlink", handleUnbindEquipment)

	// Auth (authenticated)
	mux.Handle("POST /api/user/query", auth(handleUserQuery(cfg)))
	mux.Handle("POST /api/user/logout", auth(handleLogout(cfg)))

	// Device file operations
	mux.Handle("POST /api/file/2/files/synchronous/start", auth(handleSyncStart(cfg)))
	mux.Handle("POST /api/file/2/files/synchronous/end", auth(handleSyncEnd(cfg)))
	mux.Handle("POST /api/file/2/files/create_folder_v2", auth(handleCreateFolder(cfg)))
	mux.Handle("POST /api/file/2/files/list_folder", auth(handleListFolderV2(cfg)))
	mux.Handle("POST /api/file/3/files/delete_folder_v3", auth(handleDeleteV3(cfg)))
	mux.Handle("POST /api/file/3/files/upload/apply", auth(handleUploadApply(cfg)))
	mux.Handle("POST /api/file/2/files/upload/finish", auth(handleUploadFinish(cfg)))
	mux.Handle("POST /api/file/3/files/download_v3", auth(handleDownloadV3(cfg)))
	mux.Handle("POST /api/file/3/files/query_v3", auth(handleQueryV3(cfg)))
	mux.Handle("POST /api/file/3/files/query/by/path_v3", auth(handleQueryByPathV3(cfg)))
	mux.Handle("POST /api/file/3/files/move_v3", auth(handleMoveV3(cfg)))
	mux.Handle("POST /api/file/3/files/list_folder_v3", auth(handleListFolderV3(cfg)))
	mux.Handle("POST /api/file/3/files/copy_v3", auth(handleCopyV3(cfg)))
	mux.Handle("POST /api/file/2/users/get_space_usage", auth(handleSpaceUsage(cfg)))

	// Upload/download (signature-authenticated)
	mux.HandleFunc("POST /api/oss/upload", handleOssUpload(cfg))
	mux.HandleFunc("POST /api/oss/upload/part", handleOssUploadPart(cfg))
	mux.HandleFunc("GET /api/oss/download", handleOssDownload(cfg))

	// Schedule/tasks
	mux.Handle("POST /api/file/schedule/group", auth(handleAddScheduleGroup(cfg)))
	mux.Handle("PUT /api/file/schedule/group", auth(handleUpdateScheduleGroup(cfg)))
	mux.Handle("DELETE /api/file/schedule/group/{taskListId}", auth(handleDeleteScheduleGroup(cfg)))
	mux.Handle("POST /api/file/schedule/group/all", auth(handleListScheduleGroups(cfg)))
	mux.Handle("POST /api/file/schedule/task", auth(handleAddTask(cfg)))
	mux.Handle("PUT /api/file/schedule/task/list", auth(handleBatchUpdateTasks(cfg)))
	mux.Handle("DELETE /api/file/schedule/task/{taskId}", auth(handleDeleteTask(cfg)))
	mux.Handle("POST /api/file/schedule/task/all", auth(handleListTasks(cfg)))

	// Summaries/digests
	mux.Handle("POST /api/file/add/summary/group", auth(handleAddSummaryGroup(cfg)))
	mux.Handle("PUT /api/file/update/summary/group", auth(handleUpdateSummaryGroup(cfg)))
	mux.Handle("DELETE /api/file/delete/summary/group", auth(handleDeleteSummaryGroup(cfg)))
	mux.Handle("POST /api/file/query/summary/group", auth(handleQuerySummaryGroups(cfg)))
	mux.Handle("POST /api/file/add/summary", auth(handleAddSummary(cfg)))
	mux.Handle("PUT /api/file/update/summary", auth(handleUpdateSummary(cfg)))
	mux.Handle("DELETE /api/file/delete/summary", auth(handleDeleteSummary(cfg)))
	mux.Handle("POST /api/file/query/summary/hash", auth(handleQuerySummaryHash(cfg)))
	mux.Handle("POST /api/file/query/summary/id", auth(handleQuerySummaryByIds(cfg)))
	mux.Handle("POST /api/file/query/summary", auth(handleQuerySummaries(cfg)))
	mux.Handle("POST /api/file/upload/apply/summary", auth(handleUploadSummaryFile(cfg)))
	mux.Handle("POST /api/file/download/summary", auth(handleDownloadSummary(cfg)))

	// Socket.IO
	mux.HandleFunc("/socket.io/", handleSocketIO(cfg))

	// Catch-all: log any unmatched API request so we detect removed endpoints the tablet needs.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("UNHANDLED %s %s from %s\n   Headers: %v\n   Body: %s",
			r.Method, r.URL.String(), r.RemoteAddr, r.Header, truncate(string(body), maxLogBody))
		jsonError(w, 404, ErrSystem, "endpoint not found")
	})
}
