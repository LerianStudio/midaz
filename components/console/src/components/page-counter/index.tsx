import { useIntl } from 'react-intl'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

interface PageCounterProps {
  limit: number
  setLimit: (limit: number) => void
  limitValues: number[]
}

export const PageCounter = ({
  limit,
  setLimit,
  limitValues
}: PageCounterProps) => {
  const intl = useIntl()
  return (
    <div className="flex w-full items-center gap-2">
      <div className="flex w-full justify-end gap-1">
        <p className="mt-2 mr-5 text-sm font-medium whitespace-nowrap text-gray-600">
          {intl.formatMessage({
            id: 'common.itemsPerPage',
            defaultMessage: 'Items per page'
          })}
        </p>
        <Select
          value={limit.toString()}
          onValueChange={(value) => setLimit(Number(value))}
        >
          <SelectTrigger className="w-20">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {limitValues.map((value) => (
              <SelectItem key={value} value={value.toString()}>
                {value}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
