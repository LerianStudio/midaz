export function handleActionError(error: unknown, message: string): never {
  console.error(message, error)

  if (error instanceof Error) {
    throw new Error(`${message}: ${error.message}`)
  }

  throw new Error(message)
}
