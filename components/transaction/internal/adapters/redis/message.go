package redis

const TransactionBackupQueue = "transaction_backup_queue"

// RedisMessage is a struct that represents a redis message.
type RedisMessage struct {
	ID        string `msgpack:"id"`
	Payload   any    `msgpack:"payload"`
	Timestamp int64  `msgpack:"timestamp"`
	Status    string `msgpack:"status"`
}
