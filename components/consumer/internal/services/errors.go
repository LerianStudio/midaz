package services

import "errors"

// ErrDatabaseItemNotFound is thrown a new item informed was not found
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")