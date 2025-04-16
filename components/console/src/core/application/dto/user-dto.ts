export interface CreateUserDto {
  firstName: string
  lastName: string
  email: string
  username: string
  groups: string[]
  password: string
}

export interface UpdateUserDto {
  firstName?: string
  lastName?: string
  email?: string
  username?: string
  groups?: string[]
}

export interface UserResponseDto {
  id: string
  firstName: string
  lastName: string
  email: string
  username: string
  groups: string[]
}
