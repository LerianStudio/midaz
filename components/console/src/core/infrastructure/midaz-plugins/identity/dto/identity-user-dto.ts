export type IdentityCreateUserDto = {
  firstName: string
  lastName: string
  email: string
  username: string
  groups: string[]
  password: string
}

export type IdentityUpdateUserDto = Omit<
  Partial<IdentityCreateUserDto>,
  'username' | 'password'
>

export type IdentityUserDto = Omit<IdentityCreateUserDto, 'password'> & {
  id: string
}
