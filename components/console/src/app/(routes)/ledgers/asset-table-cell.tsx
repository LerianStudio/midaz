import {
  TableCell,
  TableCellAction,
  TableCellWrapper
} from '@/components/ui/table'
import { AssetDto } from '@/core/application/dto/asset-dto'
import { useIntl } from 'react-intl'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { ExternalLink } from 'lucide-react'
import { Button } from '@/components/ui/button'

export type AssetTableCellProps = {
  assets: AssetDto[]
  onCreate?: () => void
  onClick?: () => void
}

export const AssetTableCell = ({
  assets,
  onCreate,
  onClick
}: AssetTableCellProps) => {
  const intl = useIntl()

  const codes = assets?.map((asset) => asset.code).join(', ')

  if (assets.length === 0) {
    return (
      <TableCell>
        <Button
          variant="link"
          className="text-shadcn-600 h-fit px-0 py-0 no-underline group-hover/table-cell:underline"
          onClick={(e) => {
            e.stopPropagation()
            onCreate?.()
          }}
        >
          {intl.formatMessage({
            id: 'common.add',
            defaultMessage: 'Add'
          })}
        </Button>
      </TableCell>
    )
  }

  if (assets.length <= 3) {
    return (
      <TableCell onClick={onClick}>
        <TableCellWrapper>
          <p className="cursor-pointer">{codes}</p>
          <TableCellAction>
            <ExternalLink className="size-3.5" />
          </TableCellAction>
        </TableCellWrapper>
      </TableCell>
    )
  }

  return (
    <TableCell onClick={onClick}>
      <TableCellWrapper>
        <TooltipProvider>
          <Tooltip delayDuration={300}>
            <TooltipTrigger asChild>
              <p className="cursor-pointer">
                {intl.formatMessage(
                  {
                    id: 'ledgers.assets.count',
                    defaultMessage: '{count} assets'
                  },
                  { count: assets.length }
                )}
              </p>
            </TooltipTrigger>
            <TooltipContent className="max-w-80">
              {assets.map((asset) => asset.code).join(', ')}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
        <TableCellAction>
          <ExternalLink className="size-3.5" />
        </TableCellAction>
      </TableCellWrapper>
    </TableCell>
  )
}
