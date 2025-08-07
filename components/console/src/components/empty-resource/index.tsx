import { Card, CardContent } from '../ui/card'
import Image from 'next/image'
import NoResourceImage from '@/../public/images/no-resource.png'
import { Separator } from '../ui/separator'

export type EmptyResourceProps = React.PropsWithChildren & {
  message?: string
  extra?: string
}

export const EmptyResource = ({
  message,
  extra,
  children
}: EmptyResourceProps) => {
  return (
    <Card className="gap-0 rounded-b-none p-0">
      <CardContent className="p-6">
        <div className="flex flex-col items-center justify-center gap-4">
          <Image className="mb-2" src={NoResourceImage} alt="No Resource" />

          <span className="text-shadcn-400 text-center text-sm font-medium">
            {message}
          </span>

          {children}
        </div>
      </CardContent>
      {extra && (
        <>
          <Separator />
          <div className="px-6 py-3">
            <span className="text-shadcn-400 text-sm font-normal italic">
              {extra}
            </span>
          </div>
        </>
      )}
    </Card>
  )
}
