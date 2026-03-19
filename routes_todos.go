package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func handleAddScheduleGroup(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		taskListId := bodyStr(body, "taskListId")
		title := bodyStr(body, "title")
		lastModified := bodyInt(body, "lastModified")
		createTime := bodyInt(body, "createTime")
		if createTime == 0 {
			createTime = time.Now().UnixMilli()
		}
		if taskListId == "" {
			h := md5.Sum([]byte(fmt.Sprintf("%s%d", title, lastModified)))
			taskListId = hex.EncodeToString(h[:])
			candidate := taskListId
			for i := 0; i < 100; i++ {
				var existing string
				if db.QueryRow(`SELECT task_list_id FROM schedule_groups WHERE task_list_id = ?`, candidate).Scan(&existing) == sql.ErrNoRows {
					taskListId = candidate
					break
				}
				candidate = taskListId + strconv.Itoa(i)
			}
			taskListId = candidate
		}
		_, err := db.Exec(`INSERT INTO schedule_groups (task_list_id, user_id, title, last_modified, create_time) VALUES (?, ?, ?, ?, ?)`,
			taskListId, userID, title, lastModified, createTime)
		if err != nil {
			_, _ = db.Exec(`UPDATE schedule_groups SET title = ?, last_modified = ?, updated_at = CURRENT_TIMESTAMP WHERE task_list_id = ? AND user_id = ?`,
				title, lastModified, taskListId, userID)
		}
		jsonSuccess(w, map[string]interface{}{"taskListId": taskListId})
	}
}

func handleUpdateScheduleGroup(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		_, err := db.Exec(`UPDATE schedule_groups SET title = ?, last_modified = ?, updated_at = CURRENT_TIMESTAMP WHERE task_list_id = ? AND user_id = ?`,
			bodyStr(body, "title"), bodyInt(body, "lastModified"), bodyStr(body, "taskListId"), userID)
		if err != nil {
			jsonErrorCode(w, ErrTaskGroupNotFound)
			return
		}
		jsonSuccess(w, nil)
	}
}

func handleDeleteScheduleGroup(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		taskListId := r.PathValue("taskListId")
		_, _ = db.Exec(`DELETE FROM schedule_tasks WHERE task_list_id = ? AND user_id = ?`, taskListId, userID)
		_, _ = db.Exec(`DELETE FROM schedule_groups WHERE task_list_id = ? AND user_id = ?`, taskListId, userID)
		jsonSuccess(w, nil)
	}
}

func handleListScheduleGroups(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		maxResults, _ := strconv.Atoi(bodyStr(body, "maxResults"))
		if maxResults <= 0 {
			maxResults = 20
		}
		pageToken, _ := strconv.Atoi(bodyStr(body, "pageToken"))
		if pageToken <= 0 {
			pageToken = 1
		}
		offset := (pageToken - 1) * maxResults
		rows, err := db.Query(`SELECT task_list_id, title, last_modified, create_time FROM schedule_groups WHERE user_id = ? ORDER BY create_time DESC LIMIT ? OFFSET ?`,
			userID, maxResults+1, offset)
		if err != nil {
			jsonSuccess(w, map[string]interface{}{"pageToken": nil, "scheduleTaskGroup": []interface{}{}})
			return
		}
		defer rows.Close()

		var groups []interface{}
		for rows.Next() {
			var taskListId, title string
			var lastModified, createTime int64
			rows.Scan(&taskListId, &title, &lastModified, &createTime)
			groups = append(groups, map[string]interface{}{
				"taskListId": taskListId, "userId": strconv.FormatInt(userID, 10),
				"title": title, "lastModified": lastModified, "isDeleted": "N", "createTime": createTime,
			})
		}
		if groups == nil {
			groups = []interface{}{}
		}
		var nextPage interface{}
		if len(groups) > maxResults {
			groups = groups[:maxResults]
			nextPage = strconv.Itoa(pageToken + 1)
		}
		jsonSuccess(w, map[string]interface{}{"pageToken": nextPage, "scheduleTaskGroup": groups})
	}
}

func handleAddTask(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		taskId := bodyStr(body, "taskId")
		if taskId == "" {
			taskId = generateNonce()
		}
		taskListId := bodyStr(body, "taskListId")

		if taskListId != "" {
			var gid string
			if db.QueryRow(`SELECT task_list_id FROM schedule_groups WHERE task_list_id = ? AND user_id = ?`,
				taskListId, userID).Scan(&gid) != nil {
				jsonErrorCode(w, ErrTaskGroupNotFound)
				return
			}
		}

		isReminderOn := bodyStr(body, "isReminderOn")
		if isReminderOn == "" {
			isReminderOn = "N"
		}
		status := bodyStr(body, "status")
		if status == "" {
			status = "needsAction"
		}

		_, err := db.Exec(`INSERT INTO schedule_tasks (task_id, user_id, task_list_id, title, detail, last_modified, recurrence, is_reminder_on, status, importance, due_time, completed_time, links, sort, sort_completed, planer_sort, sort_time, planer_sort_time, all_sort, all_sort_completed, all_sort_time, recurrence_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskId, userID, taskListId,
			bodyStr(body, "title"), bodyStr(body, "detail"), bodyInt(body, "lastModified"),
			bodyStr(body, "recurrence"), isReminderOn, status, bodyStr(body, "importance"),
			bodyInt(body, "dueTime"), bodyInt(body, "completedTime"), bodyStr(body, "links"),
			bodyInt(body, "sort"), bodyInt(body, "sortCompleted"), bodyInt(body, "planerSort"),
			bodyInt(body, "sortTime"), bodyInt(body, "planerSortTime"),
			bodyInt(body, "allSort"), bodyInt(body, "allSortCompleted"), bodyInt(body, "allSortTime"),
			bodyStr(body, "recurrenceId"))
		if err != nil {
			_, _ = db.Exec(`UPDATE schedule_tasks SET task_list_id=?, title=?, detail=?, last_modified=?, recurrence=?, is_reminder_on=?, status=?, importance=?, due_time=?, completed_time=?, links=?, sort=?, sort_completed=?, planer_sort=?, sort_time=?, planer_sort_time=?, all_sort=?, all_sort_completed=?, all_sort_time=?, recurrence_id=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=? AND user_id=?`,
				taskListId,
				bodyStr(body, "title"), bodyStr(body, "detail"), bodyInt(body, "lastModified"),
				bodyStr(body, "recurrence"), isReminderOn, status, bodyStr(body, "importance"),
				bodyInt(body, "dueTime"), bodyInt(body, "completedTime"), bodyStr(body, "links"),
				bodyInt(body, "sort"), bodyInt(body, "sortCompleted"), bodyInt(body, "planerSort"),
				bodyInt(body, "sortTime"), bodyInt(body, "planerSortTime"),
				bodyInt(body, "allSort"), bodyInt(body, "allSortCompleted"), bodyInt(body, "allSortTime"),
				bodyStr(body, "recurrenceId"),
				taskId, userID)
		}
		jsonSuccess(w, map[string]interface{}{"taskId": taskId})
	}
}

func handleBatchUpdateTasks(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		taskList, ok := body["updateScheduleTaskList"].([]interface{})
		if !ok || len(taskList) == 0 {
			jsonSuccess(w, nil)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, 500, ErrSystem, err.Error())
			return
		}
		for _, t := range taskList {
			task, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			var existing string
			if tx.QueryRow(`SELECT task_id FROM schedule_tasks WHERE task_id = ? AND user_id = ?`, bodyStr(task, "taskId"), userID).Scan(&existing) != nil {
				tx.Rollback()
				jsonErrorCode(w, ErrTaskNotFound)
				return
			}
		}
		tx.Commit()
		for _, t := range taskList {
			task, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			updateTaskFromBody(userID, bodyStr(task, "taskId"), task)
		}
		jsonSuccess(w, nil)
	}
}

var taskFieldMap = []struct{ json, col string }{
	{"taskListId", "task_list_id"},
	{"title", "title"}, {"detail", "detail"}, {"recurrence", "recurrence"},
	{"isReminderOn", "is_reminder_on"}, {"status", "status"}, {"importance", "importance"},
	{"links", "links"}, {"recurrenceId", "recurrence_id"},
}
var taskIntFields = []struct{ json, col string }{
	{"lastModified", "last_modified"}, {"dueTime", "due_time"}, {"completedTime", "completed_time"},
	{"sort", "sort"}, {"sortCompleted", "sort_completed"}, {"planerSort", "planer_sort"},
	{"sortTime", "sort_time"}, {"planerSortTime", "planer_sort_time"},
	{"allSort", "all_sort"}, {"allSortCompleted", "all_sort_completed"}, {"allSortTime", "all_sort_time"},
}

func updateTaskFromBody(userID int64, taskId string, body map[string]interface{}) {
	sets := []string{}
	vals := []interface{}{}
	for _, f := range taskFieldMap {
		if v, ok := body[f.json]; ok {
			sets = append(sets, f.col+" = ?")
			vals = append(vals, fmt.Sprintf("%v", v))
		}
	}
	for _, f := range taskIntFields {
		if _, ok := body[f.json]; ok {
			sets = append(sets, f.col+" = ?")
			vals = append(vals, bodyInt(body, f.json))
		}
	}
	if len(sets) == 0 {
		return
	}
	sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
	vals = append(vals, taskId, userID)
	db.Exec(fmt.Sprintf("UPDATE schedule_tasks SET %s WHERE task_id = ? AND user_id = ?", strings.Join(sets, ", ")), vals...)
}

func handleDeleteTask(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		taskId := r.PathValue("taskId")
		_, _ = db.Exec(`DELETE FROM schedule_tasks WHERE task_id = ? AND user_id = ?`, taskId, userID)
		jsonSuccess(w, nil)
	}
}

func handleListTasks(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		body := parseJSONBody(r)
		maxResults, _ := strconv.Atoi(bodyStr(body, "maxResults"))
		if maxResults <= 0 {
			maxResults = 20
		}
		pageToken, _ := strconv.Atoi(bodyStr(body, "nextPageTokens"))
		if pageToken <= 0 {
			pageToken = 1
		}
		nextSyncToken := bodyInt(body, "nextSyncToken")
		offset := (pageToken - 1) * maxResults

		var rows *sql.Rows
		var err error
		const cols = `task_id, task_list_id, title, detail, last_modified, recurrence, is_reminder_on, status, importance, due_time, completed_time, links, sort, sort_completed, planer_sort, sort_time, planer_sort_time, all_sort, all_sort_completed, all_sort_time, recurrence_id`
		if nextSyncToken > 0 {
			rows, err = db.Query(`SELECT `+cols+` FROM schedule_tasks WHERE user_id = ? AND updated_at >= ? ORDER BY last_modified DESC LIMIT ? OFFSET ?`,
				userID, time.UnixMilli(nextSyncToken), maxResults+1, offset)
		} else {
			rows, err = db.Query(`SELECT `+cols+` FROM schedule_tasks WHERE user_id = ? ORDER BY last_modified DESC LIMIT ? OFFSET ?`,
				userID, maxResults+1, offset)
		}
		if err != nil {
			jsonSuccess(w, map[string]interface{}{"nextPageToken": nil, "nextSyncToken": nil, "scheduleTask": []interface{}{}})
			return
		}
		defer rows.Close()

		var tasks []interface{}
		for rows.Next() {
			var taskId, taskListId, title, detail, recurrence, isReminderOn, status, importance, links, recurrenceId string
			var lastModified, dueTime, completedTime, sort, sortCompleted, planerSort, sortTime, planerSortTime, allSort, allSortCompleted, allSortTime int64
			rows.Scan(&taskId, &taskListId, &title, &detail, &lastModified, &recurrence, &isReminderOn, &status, &importance, &dueTime, &completedTime, &links,
				&sort, &sortCompleted, &planerSort, &sortTime, &planerSortTime, &allSort, &allSortCompleted, &allSortTime, &recurrenceId)

			tasks = append(tasks, map[string]interface{}{
				"taskId": taskId, "userId": strconv.FormatInt(userID, 10),
				"taskListId": taskListId,
				"title":      title, "detail": detail, "lastModified": lastModified,
				"recurrence": recurrence, "isReminderOn": isReminderOn,
				"status": status, "importance": importance,
				"dueTime": dueTime, "completedTime": completedTime,
				"links": links, "isDeleted": "N",
				"sort": sort, "sortCompleted": sortCompleted, "planerSort": planerSort,
				"sortTime": sortTime, "planerSortTime": planerSortTime,
				"allSort": allSort, "allSortCompleted": allSortCompleted, "allSortTime": allSortTime,
				"recurrenceId": recurrenceId, "scheduleRecurTask": []interface{}{},
			})
		}
		if tasks == nil {
			tasks = []interface{}{}
		}

		var nextPageToken interface{}
		hasMore := len(tasks) > maxResults
		if hasMore {
			tasks = tasks[:maxResults]
			nextPageToken = strconv.Itoa(pageToken + 1)
		}
		var respSyncToken interface{}
		if !hasMore {
			respSyncToken = time.Now().UnixMilli()
		}
		jsonSuccess(w, map[string]interface{}{
			"nextPageToken": nextPageToken, "nextSyncToken": respSyncToken, "scheduleTask": tasks,
		})
	}
}
