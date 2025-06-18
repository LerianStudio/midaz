interface MongoServerError extends Error {
  name: 'MongoServerError'
  code: number
  keyValue?: Record<string, any>
}
