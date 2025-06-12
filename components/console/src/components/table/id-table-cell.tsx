import { TableCell } from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { useToast } from '@/hooks/use-toast'
import { truncate } from 'lodash'
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
    <TableCell>
      <TooltipProvider>
        <Tooltip delayDuration={300}>
          <TooltipTrigger onClick={handleCopyToClipboard}>
            <p className="text-shadcn-600 underline">
              {truncate(id, { length: 16 })}
            </p>
          </TooltipTrigger>
          <TooltipContent
            className="bg-shadcn-600 border-none"
            arrowPadding={0}
          >
            <p className="text-shadcn-400">{id}</p>
            <p className="text-center text-white">
              {intl.formatMessage({
                id: 'ledgers.columnsTable.tooltipCopyText',
                defaultMessage: 'Click to copy'
              })}
            </p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </TableCell>
  )
}
