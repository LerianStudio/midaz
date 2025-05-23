export abstract class DeleteUserRepository {
  abstract delete: (userId: string) => Promise<void>
}
