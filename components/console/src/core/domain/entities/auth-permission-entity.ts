type AuthActionEntity = string

type AuthResourceEntity = string

export type AuthPermissionEntity = Record<
  AuthResourceEntity,
  AuthActionEntity[]
>
