import { useIntl } from 'react-intl'
import { Button } from '../ui/button'
import type { UsePaginationReturn } from '@/hooks/use-pagination'
import { ChevronLeft, ChevronRight } from 'lucide-react'

export type PaginationProps = UsePaginationReturn & {
  total?: number
  currentItemsCount?: number
}

export const Pagination = ({
  page,
  limit,
  total = 0,
  currentItemsCount,
  nextPage,
  previousPage
}: PaginationProps) => {
  const intl = useIntl()

  // Calculate if we can go to next page
  // If we have currentItemsCount, use it to determine if there are more items
  // Otherwise, fall back to calculating based on total and current page
  const canGoNext =
    currentItemsCount !== undefined
      ? currentItemsCount >= limit
      : page * limit < total

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
        disabled={!canGoNext}
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
