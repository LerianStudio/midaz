import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import useCustomToast from '@/hooks/use-custom-toast'
import { Arrow } from '@radix-ui/react-tooltip'
import { Copy } from 'lucide-react'
import { useIntl } from 'react-intl'

type InfoTooltipProps = {
  subtitle: string
}

export const InfoTooltip = ({ subtitle }: InfoTooltipProps) => {
  const intl = useIntl()
  const { showInfo } = useCustomToast()

  const handleCopyToClipboard = (value: string) => {
    navigator.clipboard.writeText(value)
    showInfo(
      intl.formatMessage({
        id: 'common.copyMessage',
        defaultMessage: 'Copied to clipboard!'
      })
    )
  }

  return (
    <TooltipProvider>
      <Tooltip delayDuration={300}>
        <TooltipTrigger onClick={() => handleCopyToClipboard(subtitle)}>
          <Copy size={16} className="cursor-pointer" />
        </TooltipTrigger>

        <TooltipContent className="border-none bg-shadcn-600" arrowPadding={0}>
          <p className="text-sm font-medium text-shadcn-400">{subtitle}</p>
          <p className="text-center text-white">
            {intl.formatMessage({
              id: 'common.tooltipCopyText',
              defaultMessage: 'Click to copy'
            })}
          </p>
          <Arrow height={8} width={15} />
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
