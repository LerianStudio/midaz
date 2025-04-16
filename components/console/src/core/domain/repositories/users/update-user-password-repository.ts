export abstract class UpdateUserPasswordRepository {
  abstract updatePassword: (
    userId: string,
    oldPassword: string,
    newPassword: string
  ) => Promise<void>
}
