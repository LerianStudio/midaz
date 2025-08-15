import { useIntl } from 'react-intl'
import { Button } from '../ui/button'
import type { UsePaginationReturn } from '@/hooks/use-pagination'
import { ChevronLeft, ChevronRight } from 'lucide-react'

export type PaginationProps = UsePaginationReturn & {
  total?: number
  shouldDisableNext?: boolean
}

export const Pagination = ({
  page,
  limit,
  total = 0,
  shouldDisableNext = false,
  nextPage,
  previousPage
}: PaginationProps) => {
  const intl = useIntl()

  const nextButtonDisabled = shouldDisableNext || total < limit

  return (
    <div className="flex items-center justify-end space-x-2">
      <Button
        variant="outline"
        size="sm"
        onClick={previousPage}
        disabled={page <= 1}
        icon={<ChevronLeft size={16} />}
        iconPlacement="start"
      >
        {intl.formatMessage({
          id: 'table.pagination.previous',
          defaultMessage: 'Previous'
        })}
      </Button>

      <Button
        variant="outline"
        size="sm"
        onClick={nextPage}
        disabled={nextButtonDisabled}
        icon={<ChevronRight size={16} />}
        iconPlacement="end"
      >
        {intl.formatMessage({
          id: 'table.pagination.next',
          defaultMessage: 'Next'
        })}
      </Button>
    </div>
  )
}
