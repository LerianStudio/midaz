import { ChevronDown } from 'lucide-react'
import { useIntl } from 'react-intl'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem
} from '../ui/dropdown-menu'

export type StatusButtonProps = {}

export const StatusButton = () => {
  const intl = useIntl()

  return (
    <div className="flex max-h-10 items-center gap-7">
      <span className="text-shadcn-400 text-sm font-medium">
        {intl.formatMessage({
          id: 'common.status',
          defaultMessage: 'Status'
        })}
        :
      </span>

      <DropdownMenu>
        <DropdownMenuTrigger>
          <div className="bg-de-york-600 relative flex items-center justify-center rounded-md text-sm font-medium text-white focus:outline-hidden">
            <p className="px-4">Ativo</p>

            <span className="border-l border-black/15 p-2">
              <ChevronDown size={24} />
            </span>
          </div>
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem>Inativar</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
