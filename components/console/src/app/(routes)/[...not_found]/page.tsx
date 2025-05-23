'use client'

import { Button } from '@/components/ui/button'
import Link from 'next/link'
import { useIntl } from 'react-intl'

const NotFoundPage = () => {
  const intl = useIntl()

  return (
    <div className="flex h-full flex-col items-center justify-center">
      <div className="flex flex-col justify-center gap-4 text-center">
        <h1 className="text-3xl">
          {intl.formatMessage({
            id: 'notFound.title',
            defaultMessage: 'The page you are looking for does not exist.'
          })}
        </h1>
        <h1 className="mb-4 text-2xl">
          {intl.formatMessage({
            id: 'notFound.description',
            defaultMessage: 'Try accessing another page.'
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

export default NotFoundPage
