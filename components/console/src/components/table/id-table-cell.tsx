import {
  TableCell,
  TableCellAction,
  TableCellWrapper
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { useToast } from '@/hooks/use-toast'
import { truncate } from 'lodash'
import { Copy } from 'lucide-react'
import { useIntl } from 'react-intl'

export type IdTableCellProps = {
  id?: string
}

export const IdTableCell = ({ id }: IdTableCellProps) => {
  const intl = useIntl()
  const { toast } = useToast()

  const handleCopyToClipboard = () => {
    navigator.clipboard.writeText(id!)
    toast({
      description: intl.formatMessage({
        id: 'table.toast.copyId',
        defaultMessage: 'The id has been copied to your clipboard.'
      })
    })
  }

  return (
    <TableCell onClick={handleCopyToClipboard}>
      <TableCellWrapper>
        <TooltipProvider>
          <Tooltip delayDuration={300}>
            <TooltipTrigger>{truncate(id, { length: 16 })}</TooltipTrigger>
            <TooltipContent>{id}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
        <TableCellAction>
          <Copy className="size-3.5" />
        </TableCellAction>
      </TableCellWrapper>
    </TableCell>
  )
}
