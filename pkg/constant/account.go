package constant

const (
	DefaultExternalAccountAliasPrefix = "@external/"
	ExternalAccountType               = "external"
	TimeSetLock                       = 1
	TimeSetLockBalance                = 5
	LockRetry                         = 50
	RedisTimesRetry                   = 3
	CheckAndReleaseLock               = 50
	TimeToSetAccountsInRedis          = 300 //5 minutes
)
