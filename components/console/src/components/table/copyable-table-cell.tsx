import { Copy } from 'lucide-react'
import { TableCell } from '@/components/ui/table'
import { useToast } from '@/hooks/use-toast'
import { useIntl } from 'react-intl'
import { Button } from '@/components/ui/button'

export type CopyableTableCellProps = {
  value?: string
  placeholder?: string
  maxLength?: number
  showCopyIcon?: boolean
}

export const CopyableTableCell = ({
  value,
  maxLength
}: CopyableTableCellProps) => {
  const intl = useIntl()
  const { toast } = useToast()

  const displayValue =
    value && maxLength && value.length > maxLength
      ? `${value.substring(0, maxLength)}...`
      : value

  const handleCopyToClipboard = () => {
    if (!value) return

    navigator.clipboard.writeText(value)
    toast({
      description: intl.formatMessage({
        id: 'table.toast.copyValue',
        defaultMessage: 'The value has been copied to your clipboard.'
      })
    })
  }

  return (
    <TableCell>
      <div className="flex items-center gap-2">
        <span>{displayValue}</span>

        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0 hover:bg-transparent"
          onClick={handleCopyToClipboard}
        >
          <Copy size={16} />
        </Button>
      </div>
    </TableCell>
  )
}
