import { ReactNode, useState } from 'react'
import { Collapsible } from '@/components/ui/collapsible'

type RootProps = {
  children: ReactNode
  className?: string
}

export const Root = ({ children, className }: RootProps) => {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <div className="mt-12">
      <Collapsible open={isOpen} onOpenChange={setIsOpen} className={className}>
        {children}
      </Collapsible>
    </div>
  )
}
