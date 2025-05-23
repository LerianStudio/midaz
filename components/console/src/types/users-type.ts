export type UsersType = {
  id: string
  firstName: string
  lastName: string
  username: string
  email: string
  groups: string[]
}

export type CreateUserType = Omit<UsersType, 'id'>
