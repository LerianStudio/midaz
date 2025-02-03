package constant

import "time"

const (
	DefaultExternalAccountAliasPrefix       = "@external/"
	ExternalAccountType                     = "external"
	NumberOfLockRetry                 int64 = 3
	TimeLockRetry                           = 20 * time.Millisecond
	TimeSetLock                             = 10 * time.Millisecond
	SetLockBalance                          = 15 * time.Millisecond
	CheckAndReleaseLockBalance              = 16 * time.Millisecond
	TimeToSetAccountsInRedis                = 5 * time.Minute
)
