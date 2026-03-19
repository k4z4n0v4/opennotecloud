package main

const (
	ErrAccountNotFound      = "E0018"
	ErrWrongPassword        = "E0019"
	ErrAccountLocked        = "E0045"
	ErrDeviceSyncing        = "E0078"
	ErrUserNotExist         = "E0109"
	ErrDeleteNotFound       = "E0318"
	ErrMoveNotFound         = "E0320"
	ErrDownloadNotFound     = "E0321"
	ErrNameConflict         = "E0322"
	ErrTaskGroupNotFound    = "E0328"
	ErrTaskNotFound         = "E0329"
	ErrPathEmpty            = "E0334"
	ErrUniqueIDExists       = "E0338"
	ErrSummaryGroupNotFound = "E0339"
	ErrSummaryNotFound      = "E0340"
	ErrMoveFailed           = "E0344"
	ErrCircularMove         = "E0358"
	ErrSystem               = "E0706"
	ErrNotLoggedIn          = "E0712"
	ErrRequestEmpty         = "E0739"
	ErrUploadFailed         = "E1305"
	ErrSignatureFailed      = "E1306"
)

var errorMessages = map[string]string{
	ErrAccountNotFound:      "Account does not exist",
	ErrWrongPassword:        "Password error",
	ErrAccountLocked:        "User has been locked. Please try again later!",
	ErrDeviceSyncing:        "A device is currently synchronizing. Please wait until it finishes!",
	ErrUserNotExist:         "User does not exist!",
	ErrDeleteNotFound:       "The folder or file you want to delete does not exist",
	ErrMoveNotFound:         "The folder or file you want to move/rename does not exist",
	ErrDownloadNotFound:     "This file does not exist",
	ErrNameConflict:         "A file with the same name already exists",
	ErrTaskGroupNotFound:    "The task group does not exist",
	ErrTaskNotFound:         "The root task does not exist",
	ErrPathEmpty:            "The path cannot be empty",
	ErrUniqueIDExists:       "The unique identifier already exists",
	ErrSummaryGroupNotFound: "The summary library does not exist",
	ErrSummaryNotFound:      "The summary does not exist",
	ErrMoveFailed:           "Move failed",
	ErrCircularMove:         "Cannot move a folder into itself or its subdirectory",
	ErrSystem:               "System error!",
	ErrNotLoggedIn:          "You are not logged in or your login has expired. Please log in again!",
	ErrRequestEmpty:         "The request data is empty!",
	ErrUploadFailed:         "File upload failed",
	ErrSignatureFailed:      "Signature verification failed",
}

func errMsg(code string) string {
	if msg, ok := errorMessages[code]; ok {
		return msg
	}
	return "Unknown error"
}
