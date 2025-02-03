package constant

import "time"

const (
	DefaultExternalAccountAliasPrefix       = "@external/"
	ExternalAccountType                     = "external"
	NumberOfLockRetry                 int64 = 3
	TimeLockRetry                           = 100 * time.Millisecond
	TimeSetLock                             = 100 * time.Millisecond
	SetLockBalance                          = 100 * time.Millisecond
	CheckAndReleaseLockBalance              = 100 * time.Millisecond
	TimeToSetAccountsInRedis                = 5 * time.Minute
)
