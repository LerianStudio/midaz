import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ChevronDown, ChevronUp, Code, Copy, Check } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from '@/components/ui/collapsible'
import { Badge } from '@/components/ui/badge'

interface RawTransactionDataProps {
  data: any
  title?: string
  className?: string
  defaultOpen?: boolean
}

export function RawTransactionData({
  data,
  title,
  className,
  defaultOpen = false
}: RawTransactionDataProps) {
  const intl = useIntl()
  const [isOpen, setIsOpen] = useState(defaultOpen)
  const [copied, setCopied] = useState(false)

  const formattedData = JSON.stringify(data, null, 2)

  const handleCopy = () => {
    navigator.clipboard.writeText(formattedData)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Card className={cn('overflow-hidden', className)}>
      <Collapsible open={isOpen} onOpenChange={setIsOpen}>
        <CardHeader className="cursor-pointer">
          <CollapsibleTrigger className="flex w-full items-center justify-between">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Code className="h-4 w-4" />
              {title ||
                intl.formatMessage({
                  id: 'transactions.rawData',
                  defaultMessage: 'Raw Transaction Data'
                })}
              <Badge variant="secondary" className="text-xs">
                {intl.formatMessage({
                  id: 'common.debug',
                  defaultMessage: 'Debug'
                })}
              </Badge>
            </CardTitle>
            {isOpen ? (
              <ChevronUp className="text-muted-foreground h-4 w-4" />
            ) : (
              <ChevronDown className="text-muted-foreground h-4 w-4" />
            )}
          </CollapsibleTrigger>
        </CardHeader>

        <CollapsibleContent>
          <CardContent className="pt-0">
            <div className="relative">
              <Button
                variant="ghost"
                size="sm"
                className="absolute top-2 right-2"
                onClick={handleCopy}
              >
                {copied ? (
                  <>
                    <Check className="mr-1 h-4 w-4" />
                    {intl.formatMessage({
                      id: 'common.copied',
                      defaultMessage: 'Copied to clipboard'
                    })}
                  </>
                ) : (
                  <>
                    <Copy className="mr-1 h-4 w-4" />
                    {intl.formatMessage({
                      id: 'common.copy',
                      defaultMessage: 'Copy'
                    })}
                  </>
                )}
              </Button>

              <pre className="bg-muted overflow-x-auto rounded-md p-4 text-xs">
                <code className="language-json">{formattedData}</code>
              </pre>

              <div className="text-muted-foreground mt-2 text-xs">
                {intl.formatMessage({
                  id: 'transactions.rawData.disclaimer',
                  defaultMessage:
                    'This is the raw data structure for debugging purposes.'
                })}
              </div>
            </div>
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}

/**
 * Compact version that just shows a button to view raw data in a dialog
 */
interface RawDataButtonProps {
  data: any
  className?: string
}

export function RawDataButton({ data: _data, className }: RawDataButtonProps) {
  const intl = useIntl()
  const [_showDialog, _setShowDialog] = useState(false)

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className={cn('text-xs', className)}
        onClick={() => _setShowDialog(true)}
      >
        <Code className="mr-1 h-3 w-3" />
        {intl.formatMessage({
          id: 'transactions.viewRawData',
          defaultMessage: 'View Raw Data'
        })}
      </Button>
    </>
  )
}
