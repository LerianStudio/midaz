import { useIntl } from 'react-intl'

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
    <div className="flex w-full items-center justify-between">
      <div className="flex w-full justify-end gap-2">
        <span className="text-muted-foreground text-sm">
          {intl.formatMessage({
            id: 'common.itemsPerPage',
            defaultMessage: 'Items per page'
          })}
        </span>
        <select
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
          className="rounded border px-2 py-1 text-sm"
        >
          {limitValues.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
      </div>
    </div>
  )
}
