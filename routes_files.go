package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func handleSyncStart(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")

		var lockedEquip string
		var expiresAt time.Time
		err := db.QueryRow(`SELECT equipment_no, expires_at FROM sync_locks WHERE user_id = ?`, userID).Scan(&lockedEquip, &expiresAt)
		if err == nil && time.Now().Before(expiresAt) && lockedEquip != equipmentNo {
			jsonErrorCode(w, ErrDeviceSyncing)
			return
		}
		var folderCount int
		_ = db.QueryRow(`SELECT COUNT(*) FROM files WHERE user_id = ? AND directory_id = 0 AND is_folder = 'Y'`, userID).Scan(&folderCount)

		expires := time.Now().Add(10 * time.Minute)
		_, _ = db.Exec(`INSERT OR REPLACE INTO sync_locks (user_id, equipment_no, expires_at) VALUES (?, ?, ?)`,
			userID, equipmentNo, expires)

		jsonSuccess(w, map[string]interface{}{"synType": folderCount > 0, "equipmentNo": equipmentNo})
	}
}

func handleSyncEnd(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		_, _ = db.Exec(`DELETE FROM sync_locks WHERE user_id = ? AND equipment_no = ?`, userID, equipmentNo)
		jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo})
	}
}

func handleCreateFolder(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		path := strings.TrimPrefix(bodyStr(body, "path"), "/")
		autorename := bodyBool(body, "autorename")
		if path == "" {
			jsonErrorCode(w, ErrPathEmpty)
			return
		}
		email, _, _, _ := lookupUserByID(userID)
		segments := strings.Split(path, "/")
		parentID := int64(0)
		var lastID int64
		var lastName string

		for i, seg := range segments {
			if seg == "" {
				continue
			}
			var existingID int64
			err := db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND is_folder = 'Y'`,
				userID, parentID, seg).Scan(&existingID)
			if err == sql.ErrNoRows {
				if i == len(segments)-1 && !autorename {
					var conflictID int64
					if db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ?`,
						userID, parentID, seg).Scan(&conflictID) == nil {
						jsonErrorCode(w, ErrNameConflict)
						return
					}
				}
				finalName := seg
				if i == len(segments)-1 && autorename {
					finalName = autoRename(userID, parentID, seg, 0)
				}
				newID := nextID()
				if _, err = db.Exec(`INSERT INTO files (id, user_id, directory_id, file_name, is_folder) VALUES (?, ?, ?, ?, 'Y')`,
					newID, userID, parentID, finalName); err != nil {
					jsonError(w, 500, ErrSystem, err.Error())
					return
				}
				mkdirAll(getDiskPath(email, buildFullPath(userID, parentID, finalName), cfg))
				lastID, lastName, parentID = newID, finalName, newID
			} else if err == nil {
				lastID, lastName, parentID = existingID, seg, existingID
			} else {
				jsonError(w, 500, ErrSystem, err.Error())
				return
			}
		}
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"metadata": map[string]interface{}{
				"tag": "folder", "id": SnowflakeID(lastID), "name": lastName, "path_display": path,
			},
		})
	}
}

// autoRename generates "name(1)", "name(2)" etc. excludeID=0 means no exclusion.
func autoRename(userID, parentID int64, name string, excludeID int64) string {
	candidate := name
	for i := 1; i < 1000; i++ {
		var id int64
		var err error
		if excludeID == 0 {
			err = db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ?`,
				userID, parentID, candidate).Scan(&id)
		} else {
			err = db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND id != ?`,
				userID, parentID, candidate, excludeID).Scan(&id)
		}
		if err == sql.ErrNoRows {
			return candidate
		}
		ext := filepath.Ext(name)
		candidate = fmt.Sprintf("%s(%d)%s", strings.TrimSuffix(name, ext), i, ext)
	}
	return candidate
}

func handleListFolderV2(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		path := strings.Trim(bodyStr(body, "path"), "/")
		recursive := bodyBool(body, "recursive")

		dirID := int64(0)
		if path != "" {
			var err error
			if dirID, err = resolvePathToID(userID, path); err != nil {
				jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entries": []interface{}{}})
				return
			}
		}
		jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entries": listEntriesV2(userID, dirID, path, recursive)})
	}
}

func listEntriesV2(userID, dirID int64, basePath string, recursive bool) []interface{} {
	rows, err := db.Query(`SELECT id, file_name, md5, size, is_folder, directory_id, updated_at FROM files WHERE user_id = ? AND directory_id = ? ORDER BY is_folder DESC, file_name`,
		userID, dirID)
	if err != nil {
		return []interface{}{}
	}
	defer rows.Close()

	var entries []interface{}
	for rows.Next() {
		var id, size, parentDirID int64
		var name, md5, isFolder, updatedAt string
		rows.Scan(&id, &name, &md5, &size, &isFolder, &parentDirID, &updatedAt)

		tag := "file"
		if isFolder == "Y" {
			tag = "folder"
		}
		pathDisplay := name
		if basePath != "" {
			pathDisplay = basePath + "/" + name
		}

		entries = append(entries, map[string]interface{}{
			"tag": tag, "id": SnowflakeID(id), "name": name, "path_display": pathDisplay,
			"content_hash": md5, "size": size, "lastUpdateTime": datetimeToEpochMillis(updatedAt),
			"is_downloadable": true, "parent_path": basePath,
		})
		if recursive && isFolder == "Y" {
			entries = append(entries, listEntriesV2(userID, id, pathDisplay, true)...)
		}
	}
	if entries == nil {
		entries = []interface{}{}
	}
	return entries
}

func handleDeleteV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		fileID := bodyInt(body, "id")
		email, _, _, _ := lookupUserByID(userID)

		var name, isFolder string
		var dirID int64
		err := db.QueryRow(`SELECT file_name, is_folder, directory_id FROM files WHERE id = ? AND user_id = ?`,
			fileID, userID).Scan(&name, &isFolder, &dirID)
		if err != nil {
			jsonErrorCode(w, ErrDeleteNotFound)
			return
		}
		deleteFile(userID, fileID, email, cfg)
		parentPath := buildDirPath(userID, dirID)
		pathDisplay := name
		if parentPath != "" {
			pathDisplay = parentPath + "/" + name
		}
		tag := "file"
		if isFolder == "Y" {
			tag = "folder"
		}
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"metadata":    map[string]interface{}{"tag": tag, "id": SnowflakeID(fileID), "name": name, "path_display": pathDisplay},
		})
	}
}

func deleteFile(userID, fileID int64, email string, cfg *Config) {
	var name, isFolder string
	var dirID int64
	if db.QueryRow(`SELECT file_name, is_folder, directory_id FROM files WHERE id = ? AND user_id = ?`,
		fileID, userID).Scan(&name, &isFolder, &dirID) != nil {
		return
	}
	// Collect child IDs first, then close cursor before recursing.
	rows, _ := db.Query(`SELECT id FROM files WHERE user_id = ? AND directory_id = ?`, userID, fileID)
	if rows != nil {
		var childIDs []int64
		for rows.Next() {
			var cid int64
			rows.Scan(&cid)
			childIDs = append(childIDs, cid)
		}
		rows.Close()
		for _, cid := range childIDs {
			deleteFile(userID, cid, email, cfg)
		}
	}
	diskPath := getDiskPath(email, buildFullPath(userID, dirID, name), cfg)
	_, _ = db.Exec(`DELETE FROM files WHERE id = ? AND user_id = ?`, fileID, userID)
	if isFolder == "Y" {
		os.RemoveAll(diskPath)
	} else {
		os.Remove(diskPath)
	}
}

func handleUploadApply(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		path := strings.TrimPrefix(bodyStr(body, "path"), "/")
		fileName := bodyStr(body, "fileName")

		// Strip filename from path to get directory.
		dirPath := path
		if strings.HasSuffix(dirPath, "/"+fileName) {
			dirPath = strings.TrimSuffix(dirPath, "/"+fileName)
		} else if strings.HasSuffix(dirPath, fileName) {
			if idx := strings.LastIndex(dirPath, "/"); idx >= 0 {
				dirPath = dirPath[:idx]
			}
		}
		if !strings.HasSuffix(dirPath, "/") {
			dirPath += "/"
		}

		encodedPath := fileSignPath(dirPath)
		sig, ts, nonce := generateUploadSignature(cfg.JWTSecret, encodedPath)
		base := fmt.Sprintf("%s/api/oss/upload", cfg.BaseURL)
		partBase := fmt.Sprintf("%s/api/oss/upload/part", cfg.BaseURL)
		qp := fmt.Sprintf("?signature=%s&timestamp=%s&nonce=%s&path=%s", sig, ts, nonce, encodedPath)

		jsonSuccess(w, map[string]interface{}{
			"equipmentNo":   equipmentNo,
			"innerName":     fileName,
			"fullUploadUrl": base + qp,
			"partUploadUrl": partBase + qp,
		})
	}
}

func handleUploadFinish(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		path := strings.TrimPrefix(bodyStr(body, "path"), "/")
		fileName := bodyStr(body, "fileName")
		contentHash := bodyStr(body, "content_hash")
		size, _ := strconv.ParseInt(bodyStr(body, "size"), 10, 64)

		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		email, _, _, _ := lookupUserByID(userID)
		dirID, err := resolveOrCreatePath(userID, email, strings.TrimSuffix(path, "/"), cfg)
		if err != nil {
			jsonError(w, 500, ErrSystem, err.Error())
			return
		}

		var existingID int64
		err = db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND is_folder = 'N'`,
			userID, dirID, fileName).Scan(&existingID)

		var fileID int64
		if err == nil {
			fileID = existingID
			_, _ = db.Exec(`UPDATE files SET md5 = ?, size = ?, inner_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				contentHash, size, fileName, fileID)
		} else {
			fileID = nextID()
			if _, err = db.Exec(`INSERT INTO files (id, user_id, directory_id, file_name, inner_name, md5, size, is_folder) VALUES (?, ?, ?, ?, ?, ?, ?, 'N')`,
				fileID, userID, dirID, fileName, fileName, contentHash, size); err != nil {
				jsonError(w, 500, ErrSystem, err.Error())
				return
			}
		}
		refreshSyncLock(userID)
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo":  equipmentNo,
			"path_display": path + fileName,
			"id":           SnowflakeID(fileID), "size": size, "name": fileName, "content_hash": contentHash,
		})
	}
}

func handleDownloadV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		fileID := bodyInt(body, "id")

		var name, md5, isFolder string
		var dirID, size int64
		err := db.QueryRow(`SELECT file_name, md5, is_folder, directory_id, size FROM files WHERE id = ? AND user_id = ?`,
			fileID, userID).Scan(&name, &md5, &isFolder, &dirID, &size)
		if err != nil {
			jsonErrorCode(w, ErrDownloadNotFound)
			return
		}
		email, _, _, _ := lookupUserByID(userID)
		dbPath := buildFullPath(userID, dirID, name)
		if _, err := os.Stat(getDiskPath(email, dbPath, cfg)); os.IsNotExist(err) {
			jsonErrorCode(w, ErrDownloadNotFound)
			return
		}
		encodedPath := fileSignPath(dbPath)
		sig, ts, nonce := generateUploadSignature(cfg.JWTSecret, encodedPath)
		dlURL := fmt.Sprintf("%s/api/oss/download?path=%s&signature=%s&timestamp=%s&nonce=%s&pathId=%d",
			cfg.BaseURL, encodedPath, sig, ts, nonce, userID)

		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"id":          SnowflakeID(fileID), "url": dlURL,
			"name": name, "path_display": buildDirPath(userID, dirID) + "/" + name,
			"content_hash": md5, "size": size, "is_downloadable": true,
		})
	}
}

func handleQueryV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		fileID := bodyInt(body, "id")

		var name, md5, isFolder string
		var dirID, size int64
		err := db.QueryRow(`SELECT file_name, md5, is_folder, directory_id, size FROM files WHERE id = ? AND user_id = ?`,
			fileID, userID).Scan(&name, &md5, &isFolder, &dirID, &size)
		if err != nil {
			jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entriesVO": nil})
			return
		}
		tag := "file"
		if isFolder == "Y" {
			tag = "folder"
		}
		fullPath := buildFullPath(userID, dirID, name)
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"entriesVO": map[string]interface{}{
				"tag": tag, "id": SnowflakeID(fileID), "name": name, "path_display": fullPath,
				"content_hash": md5, "size": size, "is_downloadable": true,
			},
		})
	}
}

func handleQueryByPathV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		path := strings.Trim(bodyStr(body, "path"), "/")

		if path == "" {
			jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entriesVO": nil})
			return
		}

		var parentPath, fileName string
		if idx := strings.LastIndex(path, "/"); idx < 0 {
			fileName = path
		} else {
			parentPath, fileName = path[:idx], path[idx+1:]
		}

		dirID := int64(0)
		if parentPath != "" {
			var err error
			if dirID, err = resolvePathToID(userID, parentPath); err != nil {
				jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entriesVO": nil})
				return
			}
		}

		// Try file first, then folder.
		var id, size int64
		var name, md5Str, isFolder string
		err := db.QueryRow(`SELECT id, file_name, md5, is_folder, size FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND is_folder = 'N'`,
			userID, dirID, fileName).Scan(&id, &name, &md5Str, &isFolder, &size)
		if err != nil {
			err = db.QueryRow(`SELECT id, file_name, md5, is_folder, size FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND is_folder = 'Y'`,
				userID, dirID, fileName).Scan(&id, &name, &md5Str, &isFolder, &size)
		}
		if err != nil {
			jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entriesVO": nil})
			return
		}
		tag := "file"
		if isFolder == "Y" {
			tag = "folder"
		}
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"entriesVO": map[string]interface{}{
				"tag": tag, "id": SnowflakeID(id), "name": name, "path_display": path,
				"content_hash": md5Str, "size": size, "is_downloadable": true,
			},
		})
	}
}

func handleMoveV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		fileID := bodyInt(body, "id")
		toPath := strings.Trim(bodyStr(body, "to_path"), "/")
		autorename := bodyBool(body, "autorename")
		email, _, _, _ := lookupUserByID(userID)

		var name, md5Str, isFolder string
		var dirID, size int64
		err := db.QueryRow(`SELECT file_name, md5, is_folder, directory_id, size FROM files WHERE id = ? AND user_id = ?`,
			fileID, userID).Scan(&name, &md5Str, &isFolder, &dirID, &size)
		if err != nil {
			jsonErrorCode(w, ErrMoveNotFound)
			return
		}

		var destDirPath, newName string
		if idx := strings.LastIndex(toPath, "/"); idx < 0 {
			newName = toPath
		} else {
			destDirPath, newName = toPath[:idx], toPath[idx+1:]
		}

		destDirID, err := resolveOrCreatePath(userID, email, destDirPath, cfg)
		if err != nil {
			jsonError(w, 500, ErrMoveFailed, err.Error())
			return
		}

		// Circular move check.
		if isFolder == "Y" && destDirID != 0 {
			checkID := destDirID
			for i := 0; i < 100 && checkID != 0; i++ {
				if checkID == fileID {
					jsonErrorCode(w, ErrCircularMove)
					return
				}
				var parentID int64
				if db.QueryRow(`SELECT directory_id FROM files WHERE id = ? AND user_id = ?`, checkID, userID).Scan(&parentID) != nil {
					break
				}
				checkID = parentID
			}
		}

		if !autorename {
			var conflictID int64
			if db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND id != ?`,
				userID, destDirID, newName, fileID).Scan(&conflictID) == nil {
				jsonErrorCode(w, ErrNameConflict)
				return
			}
		} else {
			newName = autoRename(userID, destDirID, newName, fileID)
		}

		// Move on disk.
		oldDiskPath := getDiskPath(email, buildFullPath(userID, dirID, name), cfg)
		newDBPath := newName
		if dp := buildDirPath(userID, destDirID); dp != "" {
			newDBPath = dp + "/" + newName
		}
		newDiskPath := getDiskPath(email, newDBPath, cfg)
		if oldDiskPath != newDiskPath {
			mkdirAll(filepath.Dir(newDiskPath))
			os.Rename(oldDiskPath, newDiskPath)
		}

		_, _ = db.Exec(`UPDATE files SET directory_id = ?, file_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, destDirID, newName, fileID)
		refreshSyncLock(userID)

		tag := "file"
		if isFolder == "Y" {
			tag = "folder"
		}
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"entriesVO": map[string]interface{}{
				"tag": tag, "id": SnowflakeID(fileID), "name": newName, "path_display": toPath,
				"content_hash": md5Str, "size": size, "is_downloadable": true,
			},
		})
	}
}

func handleSpaceUsage(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		var used int64
		_ = db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM files WHERE user_id = ? AND is_folder = 'N'`, userID).Scan(&used)
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo":  equipmentNo,
			"used":         used,
			"allocationVO": map[string]interface{}{"tag": "individual", "allocated": diskTotalBytes(cfg.DataDir)},
		})
	}
}

func handleListFolderV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		folderID := bodyInt(body, "id")
		recursive := bodyBool(body, "recursive")

		var name string
		var dirID int64
		err := db.QueryRow(`SELECT file_name, directory_id FROM files WHERE id = ? AND user_id = ? AND is_folder = 'Y'`,
			folderID, userID).Scan(&name, &dirID)
		if err != nil {
			jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entries": []interface{}{}})
			return
		}
		basePath := buildFullPath(userID, dirID, name)
		jsonSuccess(w, map[string]interface{}{"equipmentNo": equipmentNo, "entries": listEntriesV2(userID, folderID, basePath, recursive)})
	}
}

func handleCopyV3(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		equipmentNo := bodyStr(body, "equipmentNo")
		sourceID := bodyInt(body, "id")
		toPath := strings.Trim(bodyStr(body, "to_path"), "/")
		autorename := bodyBool(body, "autorename")
		email, _, _, _ := lookupUserByID(userID)

		var srcName, md5Str, isFolder string
		var srcDirID, size int64
		err := db.QueryRow(`SELECT file_name, md5, is_folder, directory_id, size FROM files WHERE id = ? AND user_id = ?`,
			sourceID, userID).Scan(&srcName, &md5Str, &isFolder, &srcDirID, &size)
		if err != nil {
			jsonErrorCode(w, ErrMoveNotFound)
			return
		}

		var destDirPath, newName string
		if idx := strings.LastIndex(toPath, "/"); idx < 0 {
			newName = toPath
		} else {
			destDirPath, newName = toPath[:idx], toPath[idx+1:]
		}
		destDirID, err := resolveOrCreatePath(userID, email, destDirPath, cfg)
		if err != nil {
			jsonError(w, 500, ErrSystem, err.Error())
			return
		}

		if !autorename {
			var conflictID int64
			if db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ?`,
				userID, destDirID, newName).Scan(&conflictID) == nil {
				jsonErrorCode(w, ErrNameConflict)
				return
			}
		} else {
			newName = autoRename(userID, destDirID, newName, 0)
		}

		var newID int64
		if isFolder == "Y" {
			newID = deepCopyFolder(userID, email, sourceID, destDirID, newName, cfg)
		} else {
			newID = nextID()
			db.Exec(`INSERT INTO files (id, user_id, directory_id, file_name, inner_name, md5, size, is_folder) VALUES (?, ?, ?, ?, ?, ?, ?, 'N')`,
				newID, userID, destDirID, newName, newName, md5Str, size)
			srcPath := getDiskPath(email, buildFullPath(userID, srcDirID, srcName), cfg)
			dstPath := getDiskPath(email, buildFullPath(userID, destDirID, newName), cfg)
			copyFileOnDisk(srcPath, dstPath)
		}

		tag := "file"
		if isFolder == "Y" {
			tag = "folder"
		}
		jsonSuccess(w, map[string]interface{}{
			"equipmentNo": equipmentNo,
			"entriesVO": map[string]interface{}{
				"tag": tag, "id": SnowflakeID(newID), "name": newName, "path_display": toPath,
				"content_hash": md5Str, "size": size, "is_downloadable": true,
			},
		})
	}
}

func deepCopyFolder(userID int64, email string, srcFolderID, destDirID int64, newName string, cfg *Config) int64 {
	newID := nextID()
	db.Exec(`INSERT INTO files (id, user_id, directory_id, file_name, is_folder) VALUES (?, ?, ?, ?, 'Y')`,
		newID, userID, destDirID, newName)
	mkdirAll(getDiskPath(email, buildFullPath(userID, destDirID, newName), cfg))

	rows, err := db.Query(`SELECT id, file_name, md5, size, is_folder FROM files WHERE user_id = ? AND directory_id = ?`, userID, srcFolderID)
	if err != nil {
		return newID
	}
	defer rows.Close()

	type child struct {
		id       int64
		name     string
		md5      string
		size     int64
		isFolder string
	}
	var children []child
	for rows.Next() {
		var c child
		rows.Scan(&c.id, &c.name, &c.md5, &c.size, &c.isFolder)
		children = append(children, c)
	}
	for _, c := range children {
		if c.isFolder == "Y" {
			deepCopyFolder(userID, email, c.id, newID, c.name, cfg)
		} else {
			childID := nextID()
			db.Exec(`INSERT INTO files (id, user_id, directory_id, file_name, inner_name, md5, size, is_folder) VALUES (?, ?, ?, ?, ?, ?, ?, 'N')`,
				childID, userID, newID, c.name, c.name, c.md5, c.size)
			srcPath := getDiskPath(email, buildFullPath(userID, srcFolderID, c.name), cfg)
			dstPath := getDiskPath(email, buildFullPath(userID, newID, c.name), cfg)
			copyFileOnDisk(srcPath, dstPath)
		}
	}
	return newID
}

func copyFileOnDisk(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	mkdirAll(filepath.Dir(dst))
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()
	io.Copy(out, in)
}

func resolvePathToID(userID int64, path string) (int64, error) {
	dirID := int64(0)
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		var id int64
		if err := db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND is_folder = 'Y'`,
			userID, dirID, seg).Scan(&id); err != nil {
			return 0, fmt.Errorf("segment %q not found: %w", seg, err)
		}
		dirID = id
	}
	return dirID, nil
}

func resolveOrCreatePath(userID int64, email, path string, cfg *Config) (int64, error) {
	if path == "" {
		return 0, nil
	}
	dirID := int64(0)
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		var id int64
		err := db.QueryRow(`SELECT id FROM files WHERE user_id = ? AND directory_id = ? AND file_name = ? AND is_folder = 'Y'`,
			userID, dirID, seg).Scan(&id)
		if err == sql.ErrNoRows {
			id = nextID()
			if _, err = db.Exec(`INSERT INTO files (id, user_id, directory_id, file_name, is_folder) VALUES (?, ?, ?, ?, 'Y')`,
				id, userID, dirID, seg); err != nil {
				return 0, err
			}
			mkdirAll(getDiskPath(email, buildFullPath(userID, dirID, seg), cfg))
		} else if err != nil {
			return 0, err
		}
		dirID = id
	}
	return dirID, nil
}

func refreshSyncLock(userID int64) {
	_, _ = db.Exec(`UPDATE sync_locks SET expires_at = datetime('now', '+10 minutes') WHERE user_id = ?`, userID)
}
