package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func handleQueryServer(w http.ResponseWriter, r *http.Request) {
	jsonSuccess(w, nil)
}

func handleQueryRandomCode(w http.ResponseWriter, r *http.Request) {
	body := parseJSONBody(r)
	account := bodyStr(body, "account")
	if account == "" {
		jsonErrorCode(w, ErrRequestEmpty)
		return
	}
	if _, _, _, err := lookupUserByEmail(account); err != nil {
		jsonErrorCode(w, ErrAccountNotFound)
		return
	}
	code, ts, err := generateChallenge(account)
	if err != nil {
		jsonError(w, 500, ErrSystem, err.Error())
		return
	}
	jsonSuccess(w, map[string]interface{}{"randomCode": code, "timestamp": ts})
}

func handleEquipmentLogin(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := parseJSONBody(r)
		account := bodyStr(body, "account")
		password := bodyStr(body, "password")
		timestamp := bodyInt(body, "timestamp")
		equipmentNo := bodyStr(body, "equipmentNo")

		if account == "" {
			jsonErrorCode(w, ErrAccountNotFound)
			return
		}
		userID, dbHash, username, err := lookupUserByEmail(account)
		if err != nil {
			jsonErrorCode(w, ErrAccountNotFound)
			return
		}
		if locked, _ := checkAccountLock(userID); locked {
			jsonErrorCode(w, ErrAccountLocked)
			return
		}
		randomCode, err := consumeChallenge(account, timestamp)
		if err != nil {
			jsonErrorCode(w, ErrWrongPassword)
			return
		}
		if !verifyEquipmentPassword(dbHash, randomCode, password) {
			count := recordLoginFailure(userID)
			jsonSuccess(w, map[string]interface{}{
				"success": false, "errorCode": ErrWrongPassword, "errorMsg": errMsg(ErrWrongPassword),
				"counts": strconv.Itoa(count),
			})
			return
		}
		resetLoginErrors(userID)
		token, err := createJWTToken(cfg, userID, equipmentNo)
		if err != nil {
			jsonError(w, 500, ErrSystem, err.Error())
			return
		}
		isBind, isBindEquipment := "N", "N"
		if equipmentNo != "" {
			_, _ = db.Exec(`INSERT INTO equipment (equipment_no, user_id, name, status) VALUES (?, ?, '', 'BINDUSER')
				ON CONFLICT(equipment_no) DO UPDATE SET user_id = ?, status = 'BINDUSER', updated_at = CURRENT_TIMESTAMP`,
				equipmentNo, userID, userID)
			isBind, isBindEquipment = "Y", "Y"
		}
		jsonSuccess(w, map[string]interface{}{
			"token": token, "counts": "0", "userName": username, "avatarsUrl": "",
			"lastUpdateTime": time.Now().Format("2006-01-02 15:04:05"),
			"isBind":         isBind, "isBindEquipment": isBindEquipment, "soldOutCount": 0,
		})
	}
}

func handleCheckUserExistsServer(w http.ResponseWriter, r *http.Request) {
	body := parseJSONBody(r)
	email := bodyStr(body, "email")
	userID, _, _, err := lookupUserByEmail(email)
	if err != nil {
		jsonSuccess(w, map[string]interface{}{"userId": 0, "dms": "ALL", "uniqueMachineId": ""})
		return
	}
	jsonSuccess(w, map[string]interface{}{"userId": userID, "dms": "ALL", "uniqueMachineId": ""})
}

func handleBindEquipment(w http.ResponseWriter, r *http.Request) {
	body := parseJSONBody(r)
	eqNo := bodyStr(body, "equipmentNo")
	name := bodyStr(body, "name")
	capacity := bodyStr(body, "totalCapacity")
	if eqNo != "" {
		_, _ = db.Exec(`UPDATE equipment SET name = ?, total_capacity = ?, status = 'BINDUSER', updated_at = CURRENT_TIMESTAMP WHERE equipment_no = ?`,
			name, capacity, eqNo)
	}
	jsonSuccess(w, nil)
}

func handleUnbindEquipment(w http.ResponseWriter, r *http.Request) {
	body := parseJSONBody(r)
	if eqNo := bodyStr(body, "equipmentNo"); eqNo != "" {
		_, _ = db.Exec(`UPDATE equipment SET user_id = NULL, status = 'ACTIVE', updated_at = CURRENT_TIMESTAMP WHERE equipment_no = ?`, eqNo)
	}
	jsonSuccess(w, nil)
}

func handleUserQuery(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		email, _, username, err := lookupUserByID(userID)
		if err != nil {
			jsonErrorCode(w, ErrUserNotExist)
			return
		}
		jsonSuccess(w, map[string]interface{}{
			"address": "", "avatarsUrl": "", "birthday": "", "education": "",
			"email": email, "hobby": "", "job": "", "personalSign": "",
			"telephone": "", "countryCode": "", "sex": "",
			"totalCapacity": strconv.FormatInt(diskTotalBytes(cfg.DataDir), 10),
			"userName":      username, "fileServer": "", "userId": userID,
		})
	}
}

func handleLogout(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := getToken(r); token != "" {
			deleteToken(token, cfg)
		}
		jsonSuccess(w, nil)
	}
}

// getDiskPath converts a DB path to a disk path.
// DB: "NOTE/Note/Work/file.note" -> Disk: "{dataDir}/{email}/Supernote/Note/Work/file.note"
func getDiskPath(email, dbPath string, cfg *Config) string {
	parts := strings.SplitN(dbPath, "/", 2)
	root := parts[0]
	var diskRelative string
	switch root {
	case "NOTE", "DOCUMENT":
		if len(parts) > 1 {
			diskRelative = parts[1]
		}
	default:
		diskRelative = dbPath
	}
	return filepath.Join(cfg.DataDir, email, "Supernote", diskRelative)
}

func mkdirAll(path string) error { return os.MkdirAll(path, 0755) }

// buildFullPath constructs the full DB path by walking up directory_id.
func buildFullPath(userID, directoryID int64, fileName string) string {
	parts := []string{}
	if fileName != "" {
		parts = append(parts, fileName)
	}
	dirID := directoryID
	for dirID != 0 {
		var name string
		var parentID int64
		if err := db.QueryRow(`SELECT file_name, directory_id FROM files WHERE id = ? AND user_id = ?`, dirID, userID).Scan(&name, &parentID); err != nil {
			break
		}
		parts = append([]string{name}, parts...)
		dirID = parentID
	}
	return strings.Join(parts, "/")
}

func buildDirPath(userID, directoryID int64) string {
	return buildFullPath(userID, directoryID, "")
}
