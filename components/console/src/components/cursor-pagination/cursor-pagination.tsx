import React from 'react'
import { Button } from '@/components/ui/button'
import { ChevronLeft, ChevronRight } from 'lucide-react'

interface CursorPaginationProps {
  hasNext: boolean
  hasPrev: boolean
  onNext: () => void
  onPrevious: () => void
  isLoading?: boolean
  className?: string
}

export const CursorPagination: React.FC<CursorPaginationProps> = ({
  hasNext,
  hasPrev,
  onNext,
  onPrevious,
  isLoading = false,
  className = ''
}) => {
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <Button
        variant="outline"
        size="sm"
        onClick={onPrevious}
        disabled={isLoading || !hasPrev}
      >
        <ChevronLeft className="h-4 w-4" />
        Previous
      </Button>

      <Button
        variant="outline"
        size="sm"
        onClick={onNext}
        disabled={isLoading || !hasNext}
      >
        Next
        <ChevronRight className="h-4 w-4" />
      </Button>
    </div>
  )
}
