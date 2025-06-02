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
  href?: string
}

export const CollapsibleInfo = ({
  question,
  answer,
  seeMore,
  href
}: CollapsibleInfoProps) => (
  <CollapsibleContent>
    <div className="flex w-full justify-between">
      <div className="mt-12 flex flex-col gap-3">
        <h1 className="text-xl font-bold text-[#3f3f46]">{question}</h1>

        <div className="flex items-center gap-3">
          <p className="text-shadcn-500 text-sm leading-none font-medium">
            {answer}
          </p>

          <div className="flex items-center gap-1">
            <a
              target="_blank"
              rel="noopener noreferrer"
              href={href}
              className="text-shadcn-600 justify-start text-sm font-medium underline underline-offset-4"
            >
              {seeMore}
            </a>
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
