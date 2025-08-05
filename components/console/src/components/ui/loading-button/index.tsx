import React from 'react'
import { Button, ButtonProps } from '../button'
import { Loader2 } from 'lucide-react'

export type LoadingButtonProps = ButtonProps & {
  loading?: boolean
}

function LoadingButton({
  loading,
  disabled,
  children,
  ...props
}: LoadingButtonProps) {
  return (
    <Button {...props} disabled={loading || disabled}>
      {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
      {children}
    </Button>
  )
}

export { LoadingButton }
