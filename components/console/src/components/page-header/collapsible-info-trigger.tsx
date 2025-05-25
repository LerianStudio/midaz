import { HelpCircle } from 'lucide-react'
import { Button } from '../ui/button'
import { CollapsibleTrigger } from '../ui/collapsible'

export type CollapsibleInfoTrigger = {
  question: string
}

export const CollapsibleInfoTrigger = ({
  question
}: CollapsibleInfoTrigger) => {
  return (
    <CollapsibleTrigger asChild>
      <Button
        variant="link"
        className="flex gap-2 pr-0"
        icon={<HelpCircle className="h-4 w-4" />}
        iconPlacement="end"
      >
        <span className="text-sm font-medium text-[#3f3f46]">{question}</span>
      </Button>
    </CollapsibleTrigger>
  )
}
