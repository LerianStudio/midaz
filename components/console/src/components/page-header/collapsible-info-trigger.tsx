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
      <Button variant="link" className="flex gap-2 pr-0">
        <span className="text-sm font-medium text-[#3f3f46]">{question}</span>
        <HelpCircle className="h-4 w-4" />
      </Button>
    </CollapsibleTrigger>
  )
}
