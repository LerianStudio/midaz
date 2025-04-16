import {
  CollapsibleContent,
  CollapsibleTrigger
} from '@/components/ui/collapsible'
import { Button } from '../ui/button'
import { ChevronUp, ExternalLink } from 'lucide-react'

type CollapsibleInfoProps = {
  question?: string
  answer?: string
  seeMore?: string
}

export const CollapsibleInfo = ({
  question,
  answer,
  seeMore
}: CollapsibleInfoProps) => (
  <CollapsibleContent>
    <div className="flex w-full justify-between">
      <div className="mt-12 flex flex-col gap-3">
        <h1 className="text-xl font-bold text-[#3f3f46]">{question}</h1>

        <div className="flex items-center gap-3">
          <p className="text-sm font-medium leading-none text-shadcn-500">
            {answer}
          </p>

          <div className="flex items-center gap-1">
            <Button variant="link" size="link">
              {seeMore}
            </Button>
            <ExternalLink size={16} />
          </div>
        </div>
      </div>

      <CollapsibleTrigger asChild>
        <Button variant="plain" className="cursor-pointer self-start">
          <ChevronUp size={24} className="text-shadcn-500" />
        </Button>
      </CollapsibleTrigger>
    </div>
  </CollapsibleContent>
)
