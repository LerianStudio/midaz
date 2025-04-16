type AuthResourceDto = string
type AuthActionDto = string

export type AuthPermissionResponseDto = Record<AuthResourceDto, AuthActionDto[]>
