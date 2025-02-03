package constant

import "time"

const (
	DefaultExternalAccountAliasPrefix       = "@external/"
	ExternalAccountType                     = "external"
	RedisTimesRetry                   int64 = 3
	TimeSetLock                             = 10 * time.Millisecond
	TimeSetLockBalance                      = 15 * time.Millisecond
	LockRetry                               = 20 * time.Millisecond
	CheckAndReleaseLock                     = 25 * time.Millisecond
	TimeToSetAccountsInRedis                = 5 * time.Minute
)
