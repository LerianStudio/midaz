import {
  CreateUser,
  CreateUserUseCase
} from '@/core/application/use-cases/users/create-user-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  DeleteUser,
  DeleteUserUseCase
} from '@/core/application/use-cases/users/delete-user-use-case'
import {
  FetchAllUsers,
  FetchAllUsersUseCase
} from '@/core/application/use-cases/users/fetch-all-users-use-case'
import {
  FetchUserById,
  FetchUserByIdUseCase
} from '@/core/application/use-cases/users/fetch-user-by-id-use-case'
import {
  UpdateUserPassword,
  UpdateUserPasswordUseCase
} from '@/core/application/use-cases/users/update-user-password-use-case'
import {
  UpdateUser,
  UpdateUserUseCase
} from '@/core/application/use-cases/users/update-user-use-case'
import {
  ResetUserPassword,
  ResetUserPasswordUseCase
} from '@/core/application/use-cases/users/reset-user-password-use-case'

export const UserUseCaseModule = new ContainerModule((container: Container) => {
  container.bind<CreateUser>(CreateUserUseCase).toSelf()
  container.bind<DeleteUser>(DeleteUserUseCase).toSelf()
  container.bind<FetchAllUsers>(FetchAllUsersUseCase).toSelf()
  container.bind<FetchUserById>(FetchUserByIdUseCase).toSelf()
  container.bind<UpdateUser>(UpdateUserUseCase).toSelf()
  container.bind<UpdateUserPassword>(UpdateUserPasswordUseCase).toSelf()
  container.bind<ResetUserPassword>(ResetUserPasswordUseCase).toSelf()
})
