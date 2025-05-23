import React from 'react'
import { Button, ButtonProps } from '../button'
import { Loader2 } from 'lucide-react'

export type LoadingButtonProps = ButtonProps & {
  loading?: boolean
}

const LoadingButton = React.forwardRef<HTMLButtonElement, LoadingButtonProps>(
  ({ loading, disabled, children, ...props }, ref) => {
    return (
      <Button ref={ref} {...props} disabled={loading || disabled}>
        {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        {children}
      </Button>
    )
  }
)
LoadingButton.displayName = 'LoadingButton'

export { LoadingButton }
