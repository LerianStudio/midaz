import Link from 'next/link'
import { usePermissions } from './permission-provider-client'
import { useIntl } from 'react-intl'
import { Button } from '@/components/ui/button'

type RouteEnforcerProps = React.PropsWithChildren & {
  resource: string
  action: string
}

export const RouteEnforcer = ({
  resource,
  action,
  children
}: RouteEnforcerProps) => {
  const intl = useIntl()
  const { validate } = usePermissions()

  if (!validate(resource, action)) {
    return (
      <div className="flex h-full flex-col items-center justify-center">
        <div className="flex flex-col justify-center gap-4 text-center">
          <h1 className="text-3xl">
            {intl.formatMessage({
              id: 'notAuthorized.title',
              defaultMessage: 'You are not authorized to access this page.'
            })}
          </h1>
          <div className="flex justify-center">
            <Link href="/">
              <Button>
                {intl.formatMessage({
                  id: 'notFound.backToHome',
                  defaultMessage: 'Back to Home'
                })}
              </Button>
            </Link>
          </div>
        </div>
      </div>
    )
  }

  return children
}
