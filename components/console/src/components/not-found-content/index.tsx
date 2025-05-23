import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import { Button } from '../ui/button'

export type NotFoundContentProps = {
  title: string
}

export const NotFoundContent = ({ title }: NotFoundContentProps) => {
  const intl = useIntl()
  const router = useRouter()

  return (
    <div className="flex h-full flex-col items-center justify-center">
      <div className="flex flex-col justify-center gap-8 text-center">
        <h1 className="text-3xl">{title}</h1>
        <Button onClick={() => router.back()}>
          {intl.formatMessage({
            id: 'notFoundContent.goBack',
            defaultMessage: 'Go Back'
          })}
        </Button>
      </div>
    </div>
  )
}
