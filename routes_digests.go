package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func handleAddSummaryGroup(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		uniqueID := bodyStr(body, "uniqueIdentifier")
		creationTime := bodyInt(body, "creationTime")
		lastModifiedTime := bodyInt(body, "lastModifiedTime")
		if creationTime == 0 {
			creationTime = time.Now().UnixMilli()
		}
		if lastModifiedTime == 0 {
			lastModifiedTime = time.Now().UnixMilli()
		}
		var existing int64
		if db.QueryRow(`SELECT id FROM summaries WHERE user_id = ? AND unique_identifier = ?`, userID, uniqueID).Scan(&existing) == nil {
			jsonErrorCode(w, ErrUniqueIDExists)
			return
		}
		id := nextID()
		_, err := db.Exec(`INSERT INTO summaries (id, user_id, unique_identifier, name, description, md5_hash, is_summary_group, creation_time, last_modified_time) VALUES (?, ?, ?, ?, ?, ?, 'Y', ?, ?)`,
			id, userID, uniqueID, bodyStr(body, "name"), bodyStr(body, "description"), bodyStr(body, "md5Hash"), creationTime, lastModifiedTime)
		if err != nil {
			jsonError(w, 500, ErrSystem, err.Error())
			return
		}
		jsonSuccess(w, map[string]interface{}{"id": id})
	}
}

func handleUpdateSummaryGroup(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		id := bodyInt(body, "id")
		var existing int64
		if db.QueryRow(`SELECT id FROM summaries WHERE id = ? AND user_id = ? AND is_summary_group = 'Y'`, id, userID).Scan(&existing) != nil {
			jsonErrorCode(w, ErrSummaryGroupNotFound)
			return
		}
		updates, args := buildSummaryUpdates(body, []fieldMap{
			{"uniqueIdentifier", "unique_identifier"}, {"name", "name"}, {"description", "description"},
			{"metadata", "metadata"}, {"commentStr", "comment_str"},
			{"commentHandwriteName", "comment_handwrite_name"}, {"handwriteInnerName", "handwrite_inner_name"},
			{"md5Hash", "md5_hash"},
		})
		if v := bodyInt(body, "lastModifiedTime"); v > 0 {
			updates = append(updates, "last_modified_time = ?")
			args = append(args, v)
		}
		args = append(args, id, userID)
		db.Exec(fmt.Sprintf("UPDATE summaries SET %s WHERE id = ? AND user_id = ?", strings.Join(updates, ", ")), args...)
		jsonSuccess(w, nil)
	}
}

func handleDeleteSummaryGroup(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		id := bodyInt(parseJSONBody(r), "id")
		_, _ = db.Exec(`DELETE FROM summaries WHERE id = ? AND user_id = ? AND is_summary_group = 'Y'`, id, userID)
		jsonSuccess(w, nil)
	}
}

func handleQuerySummaryGroups(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		page, size := int(bodyInt(body, "page")), int(bodyInt(body, "size"))

		var total int
		_ = db.QueryRow(`SELECT COUNT(*) FROM summaries WHERE user_id = ? AND is_summary_group = 'Y'`, userID).Scan(&total)

		var rows *sql.Rows
		var err error
		if page <= 0 || size <= 0 {
			rows, err = db.Query(summarySelectCols+` WHERE user_id = ? AND is_summary_group = 'Y' ORDER BY creation_time DESC`, userID)
		} else {
			rows, err = db.Query(summarySelectCols+` WHERE user_id = ? AND is_summary_group = 'Y' ORDER BY creation_time DESC LIMIT ? OFFSET ?`, userID, size, (page-1)*size)
		}
		if err != nil {
			jsonSuccess(w, paginatedResponse("summaryDOList", []interface{}{}, page, size, total))
			return
		}
		defer rows.Close()

		totalPages, currentPage, pageSize := 1, 1, total
		if size > 0 {
			totalPages = (total + size - 1) / size
			currentPage, pageSize = page, size
		}
		jsonSuccess(w, map[string]interface{}{
			"totalRecords": total, "totalPages": totalPages, "currentPage": currentPage,
			"pageSize": pageSize, "summaryDOList": scanSummaryRows(rows, userID),
		})
	}
}

func handleAddSummary(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		id := nextID()
		_, err := db.Exec(`INSERT INTO summaries (id, user_id, unique_identifier, name, description, file_id, parent_unique_identifier, content, data_source, source_path, source_type, tags, md5_hash, metadata, comment_str, comment_handwrite_name, handwrite_inner_name, handwrite_md5, is_summary_group, author, creation_time, last_modified_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'N', ?, ?, ?)`,
			id, userID, bodyStr(body, "uniqueIdentifier"), bodyStr(body, "name"), bodyStr(body, "description"),
			bodyInt(body, "fileId"), bodyStr(body, "parentUniqueIdentifier"), bodyStr(body, "content"),
			bodyStr(body, "dataSource"), bodyStr(body, "sourcePath"), bodyInt(body, "sourceType"),
			bodyStr(body, "tags"), bodyStr(body, "md5Hash"), bodyStr(body, "metadata"),
			bodyStr(body, "commentStr"), bodyStr(body, "commentHandwriteName"),
			bodyStr(body, "handwriteInnerName"), bodyStr(body, "handwriteMD5"),
			bodyStr(body, "author"), bodyInt(body, "creationTime"), bodyInt(body, "lastModifiedTime"))
		if err != nil {
			jsonError(w, 500, ErrSystem, err.Error())
			return
		}
		jsonSuccess(w, map[string]interface{}{"id": id})
	}
}

func handleUpdateSummary(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		id := bodyInt(body, "id")
		var existing int64
		if db.QueryRow(`SELECT id FROM summaries WHERE id = ? AND user_id = ? AND is_summary_group = 'N'`, id, userID).Scan(&existing) != nil {
			jsonErrorCode(w, ErrSummaryNotFound)
			return
		}
		updates, args := buildSummaryUpdates(body, []fieldMap{
			{"uniqueIdentifier", "unique_identifier"}, {"name", "name"}, {"description", "description"},
			{"parentUniqueIdentifier", "parent_unique_identifier"}, {"content", "content"},
			{"dataSource", "data_source"}, {"sourcePath", "source_path"},
			{"tags", "tags"}, {"md5Hash", "md5_hash"}, {"metadata", "metadata"},
			{"commentStr", "comment_str"}, {"commentHandwriteName", "comment_handwrite_name"},
			{"handwriteInnerName", "handwrite_inner_name"}, {"handwriteMD5", "handwrite_md5"},
			{"author", "author"},
		})
		for _, f := range []struct{ json, col string }{{"sourceType", "source_type"}, {"fileId", "file_id"}} {
			if _, ok := body[f.json]; ok {
				updates = append(updates, f.col+" = ?")
				args = append(args, bodyInt(body, f.json))
			}
		}
		for _, f := range []string{"lastModifiedTime", "creationTime"} {
			if v := bodyInt(body, f); v > 0 {
				col := "last_modified_time"
				if f == "creationTime" {
					col = "creation_time"
				}
				updates = append(updates, col+" = ?")
				args = append(args, v)
			}
		}
		args = append(args, id, userID)
		db.Exec(fmt.Sprintf("UPDATE summaries SET %s WHERE id = ? AND user_id = ?", strings.Join(updates, ", ")), args...)
		jsonSuccess(w, nil)
	}
}

func handleDeleteSummary(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		id := bodyInt(parseJSONBody(r), "id")
		_, _ = db.Exec(`DELETE FROM summaries WHERE id = ? AND user_id = ? AND is_summary_group = 'N'`, id, userID)
		jsonSuccess(w, nil)
	}
}

func handleQuerySummaryHash(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		page, size := int(bodyInt(body, "page")), int(bodyInt(body, "size"))
		parentUID := bodyStr(body, "parentUniqueIdentifier")

		where := `user_id = ? AND is_summary_group = 'N'`
		args := []interface{}{userID}
		if parentUID != "" {
			where += ` AND parent_unique_identifier = ?`
			args = append(args, parentUID)
		}

		var total int
		_ = db.QueryRow(`SELECT COUNT(*) FROM summaries WHERE `+where, args...).Scan(&total)

		var rows *sql.Rows
		var err error
		if page <= 0 || size <= 0 {
			rows, err = db.Query(`SELECT id, md5_hash, handwrite_md5, comment_handwrite_name, last_modified_time, metadata FROM summaries WHERE `+where+` ORDER BY creation_time DESC`, args...)
		} else {
			rows, err = db.Query(`SELECT id, md5_hash, handwrite_md5, comment_handwrite_name, last_modified_time, metadata FROM summaries WHERE `+where+` ORDER BY creation_time DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
		}
		if err != nil {
			jsonSuccess(w, paginatedResponse("summaryInfoVOList", []interface{}{}, page, size, total))
			return
		}
		defer rows.Close()

		var list []interface{}
		for rows.Next() {
			var id, lastModified int64
			var md5Hash, hwMD5, commentHW, metadata string
			rows.Scan(&id, &md5Hash, &hwMD5, &commentHW, &lastModified, &metadata)
			list = append(list, map[string]interface{}{
				"id": id, "userId": userID, "md5Hash": md5Hash, "handwriteMd5": hwMD5,
				"commentHandwriteName": commentHW, "lastModifiedTime": lastModified,
				"metadataMap": map[string]interface{}{"author": ""},
			})
		}
		if list == nil {
			list = []interface{}{}
		}

		totalPages, currentPage, pageSize := 1, 1, total
		if size > 0 {
			totalPages = (total + size - 1) / size
			currentPage, pageSize = page, size
		}
		jsonSuccess(w, map[string]interface{}{
			"totalRecords": total, "totalPages": totalPages, "currentPage": currentPage,
			"pageSize": pageSize, "summaryInfoVOList": list,
		})
	}
}

func handleUploadSummaryFile(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := parseJSONBody(r)
		fileName := bodyStr(body, "fileName")
		encodedPath := fileSignPath("summaries/")
		sig, ts, nonce := generateUploadSignature(cfg.JWTSecret, encodedPath)
		qp := fmt.Sprintf("?signature=%s&timestamp=%s&nonce=%s&path=%s", sig, ts, nonce, encodedPath)
		jsonSuccess(w, map[string]interface{}{
			"fullUploadUrl": cfg.BaseURL + "/api/oss/upload" + qp,
			"partUploadUrl": cfg.BaseURL + "/api/oss/upload/part" + qp,
			"innerName":     fileName,
		})
	}
}

func handleQuerySummaryByIds(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		page, size := int(bodyInt(body, "page")), int(bodyInt(body, "size"))

		rawIDs, ok := body["ids"].([]interface{})
		if !ok || len(rawIDs) == 0 {
			jsonSuccess(w, paginatedResponse("summaryDOList", []interface{}{}, page, size, 0))
			return
		}
		placeholders := make([]string, len(rawIDs))
		args := make([]interface{}, len(rawIDs))
		for i, v := range rawIDs {
			placeholders[i] = "?"
			args[i] = bodyInt(map[string]interface{}{"v": v}, "v")
		}
		where := fmt.Sprintf("id IN (%s) AND user_id = ? AND is_summary_group = 'N'", strings.Join(placeholders, ","))
		args = append(args, userID)

		var total int
		_ = db.QueryRow(`SELECT COUNT(*) FROM summaries WHERE `+where, args...).Scan(&total)

		var rows *sql.Rows
		var err error
		if page <= 0 || size <= 0 {
			rows, err = db.Query(summarySelectCols+` WHERE `+where+` ORDER BY creation_time DESC`, args...)
		} else {
			rows, err = db.Query(summarySelectCols+` WHERE `+where+` ORDER BY creation_time DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
		}
		if err != nil {
			jsonSuccess(w, paginatedResponse("summaryDOList", []interface{}{}, page, size, total))
			return
		}
		defer rows.Close()

		totalPages, currentPage, pageSize := 1, 1, total
		if size > 0 {
			totalPages = (total + size - 1) / size
			currentPage, pageSize = page, size
		}
		jsonSuccess(w, map[string]interface{}{
			"totalRecords": total, "totalPages": totalPages, "currentPage": currentPage,
			"pageSize": pageSize, "summaryDOList": scanSummaryRows(rows, userID),
		})
	}
}

func handleQuerySummaries(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		page, size := int(bodyInt(body, "page")), int(bodyInt(body, "size"))
		parentUID := bodyStr(body, "parentUniqueIdentifier")

		where := `user_id = ? AND is_summary_group = 'N'`
		args := []interface{}{userID}
		if parentUID != "" {
			where += ` AND parent_unique_identifier = ?`
			args = append(args, parentUID)
		}

		var total int
		_ = db.QueryRow(`SELECT COUNT(*) FROM summaries WHERE `+where, args...).Scan(&total)

		var rows *sql.Rows
		var err error
		if page <= 0 || size <= 0 {
			rows, err = db.Query(summarySelectCols+` WHERE `+where+` ORDER BY creation_time DESC`, args...)
		} else {
			rows, err = db.Query(summarySelectCols+` WHERE `+where+` ORDER BY creation_time DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
		}
		if err != nil {
			jsonSuccess(w, paginatedResponse("summaryDOList", []interface{}{}, page, size, total))
			return
		}
		defer rows.Close()

		totalPages, currentPage, pageSize := 1, 1, total
		if size > 0 {
			totalPages = (total + size - 1) / size
			currentPage, pageSize = page, size
		}
		jsonSuccess(w, map[string]interface{}{
			"totalRecords": total, "totalPages": totalPages, "currentPage": currentPage,
			"pageSize": pageSize, "summaryDOList": scanSummaryRows(rows, userID),
		})
	}
}

func handleDownloadSummary(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		id := bodyInt(body, "id")

		var innerName string
		err := db.QueryRow(`SELECT handwrite_inner_name FROM summaries WHERE id = ? AND user_id = ?`, id, userID).Scan(&innerName)
		if err != nil || innerName == "" {
			jsonErrorCode(w, ErrSummaryNotFound)
			return
		}
		encodedPath := fileSignPath("summaries/" + innerName)
		sig, ts, nonce := generateUploadSignature(cfg.JWTSecret, encodedPath)
		dlURL := fmt.Sprintf("%s/api/oss/download?path=%s&signature=%s&timestamp=%s&nonce=%s",
			cfg.BaseURL, encodedPath, sig, ts, nonce)
		jsonSuccess(w, map[string]interface{}{"url": dlURL})
	}
}

// Helpers

type fieldMap struct{ json, col string }

const summarySelectCols = `SELECT id, unique_identifier, name, description, file_id, parent_unique_identifier, content, data_source, source_path, source_type, tags, md5_hash, metadata, comment_str, comment_handwrite_name, handwrite_inner_name, handwrite_md5, is_summary_group, author, creation_time, last_modified_time, created_at, updated_at FROM summaries`

func buildSummaryUpdates(body map[string]interface{}, fields []fieldMap) ([]string, []interface{}) {
	updates := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []interface{}{}
	for _, f := range fields {
		if v, ok := body[f.json]; ok && v != nil {
			updates = append(updates, f.col+" = ?")
			args = append(args, fmt.Sprintf("%v", v))
		}
	}
	return updates, args
}

func scanSummaryRows(rows *sql.Rows, userID int64) []interface{} {
	var list []interface{}
	for rows.Next() {
		var id, fileId, sourceType, creationTime, lastModifiedTime int64
		var uniqueID, name, description, parentUID, content, dataSource, sourcePath, tags, md5Hash, metadata, commentStr, commentHW, hwInnerName, hwMD5, isSummaryGroup, author string
		var createdAt, updatedAt time.Time
		rows.Scan(&id, &uniqueID, &name, &description, &fileId, &parentUID, &content, &dataSource, &sourcePath, &sourceType, &tags, &md5Hash, &metadata, &commentStr, &commentHW, &hwInnerName, &hwMD5, &isSummaryGroup, &author, &creationTime, &lastModifiedTime, &createdAt, &updatedAt)
		list = append(list, map[string]interface{}{
			"id": id, "userId": userID, "uniqueIdentifier": uniqueID, "name": name,
			"description": description, "fileId": fileId, "parentUniqueIdentifier": parentUID,
			"content": content, "dataSource": dataSource, "sourcePath": sourcePath,
			"sourceType": sourceType, "tags": tags, "md5Hash": md5Hash, "metadata": metadata,
			"commentStr": commentStr, "commentHandwriteName": commentHW,
			"handwriteInnerName": hwInnerName, "handwriteMD5": hwMD5,
			"isSummaryGroup": isSummaryGroup, "isDeleted": "N", "creationTime": creationTime,
			"lastModifiedTime": lastModifiedTime, "author": author,
			"createTime": createdAt.Format("2006-01-02 15:04:05"),
			"updateTime": updatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	if list == nil {
		list = []interface{}{}
	}
	return list
}

func paginatedResponse(key string, list []interface{}, page, size, total int) map[string]interface{} {
	return map[string]interface{}{
		"totalRecords": total, "totalPages": 0, "currentPage": page, "pageSize": size, key: list,
	}
}
