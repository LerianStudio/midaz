export type CreateUserDto = {
  firstName: string
  lastName: string
  email: string
  username: string
  groups: string[]
  password: string
}

export type UpdateUserDto = Omit<Partial<CreateUserDto>, 'password'>

export type UserResponseDto = {
  id: string
  firstName: string
  lastName: string
  email: string
  username: string
  groups: string[]
}
