import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { useToast } from '@/hooks/use-toast'
import { Arrow } from '@radix-ui/react-tooltip'
import { Copy } from 'lucide-react'
import { useIntl } from 'react-intl'

type InfoTooltipProps = {
  subtitle: string
}

export const InfoTooltip = ({ subtitle }: InfoTooltipProps) => {
  const intl = useIntl()
  const { toast } = useToast()

  const handleCopyToClipboard = (value: string) => {
    navigator.clipboard.writeText(value)
    toast({
      description: intl.formatMessage({
        id: 'common.copyMessage',
        defaultMessage: 'Copied to clipboard!'
      })
    })
  }

  return (
    <TooltipProvider>
      <Tooltip delayDuration={300}>
        <TooltipTrigger onClick={() => handleCopyToClipboard(subtitle)}>
          <Copy size={16} className="cursor-pointer" />
        </TooltipTrigger>

        <TooltipContent className="bg-shadcn-600 border-none" arrowPadding={0}>
          <p className="text-shadcn-400 text-sm font-medium">{subtitle}</p>
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
