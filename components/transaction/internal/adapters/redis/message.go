package redis

const TransactionBackupQueue = "backup_queue:{transactions}"

// RedisMessage is a struct that represents a redis message.
type RedisMessage struct {
	HeaderID    string `msgpack:"header_id"`
	Traceparent string `msgpack:"traceparent"`
	ID          string `msgpack:"id"`
	Payload     any    `msgpack:"payload"`
	Timestamp   int64  `msgpack:"timestamp"`
	Status      string `msgpack:"status"`
}
