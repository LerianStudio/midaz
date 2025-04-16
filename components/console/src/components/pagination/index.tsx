import { useIntl } from 'react-intl'
import { Button } from '../ui/button'
import type { UsePaginationReturn } from '@/hooks/use-pagination'
import { ChevronLeft, ChevronRight } from 'lucide-react'

export type PaginationProps = UsePaginationReturn & {
  total?: number
}

export const Pagination = ({
  page,
  limit,
  total = 0,
  nextPage,
  previousPage
}: PaginationProps) => {
  const intl = useIntl()

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
        disabled={total < limit}
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
