'use client'

import { usePermissions } from './permission-provider-client'

type EnforceProps = React.PropsWithChildren & {
  resource: string
  action: string
}

export const Enforce = ({ resource, action, children }: EnforceProps) => {
  const { validate } = usePermissions()

  if (!validate || !validate(resource, action)) {
    return null
  }

  return children
}
