export abstract class ResetUserPasswordRepository {
  abstract resetPassword: (userId: string, newPassword: string) => Promise<void>
}
