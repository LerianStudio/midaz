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
import { useRouter } from 'next/navigation'

export type AssetTableCellProps = {
  assets: AssetDto[]
  onCreate?: () => void
}

export const AssetTableCell = ({ assets, onCreate }: AssetTableCellProps) => {
  const intl = useIntl()
  const router = useRouter()

  const codes = assets?.map((asset) => asset.code).join(', ')

  const handleClick = () => router.push('/assets')

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
      <TableCell onClick={handleClick}>
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
    <TableCell onClick={handleClick}>
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
