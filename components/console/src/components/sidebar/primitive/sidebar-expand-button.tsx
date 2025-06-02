'use client'

import { PanelLeftClose, PanelRightClose } from 'lucide-react'
import { Button } from '../../ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '../../ui/tooltip'
import { useIntl } from 'react-intl'
import { SidebarFooter, useSidebar } from '.'
import React from 'react'

export const SidebarExpandButton = () => {
  const intl = useIntl()
  const { isCollapsed, toggleSidebar } = useSidebar()

  return (
    <React.Fragment>
      {!isCollapsed && (
        <div className="border-shadcn-200 flex w-full bg-white">
          <div className="absolute right-[-20px] bottom-4">
            <Button
              variant="white"
              className="border-shadcn-200 rounded-full border p-2"
              onClick={toggleSidebar}
            >
              <PanelLeftClose className="text-shadcn-400" />
            </Button>
          </div>
        </div>
      )}

      {isCollapsed && (
        <SidebarFooter>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger
                className="group/expand-button text-shadcn-400 hover:bg-accent rounded-sm bg-transparent p-2"
                onClick={toggleSidebar}
              >
                <PanelRightClose className="group-hover/expand-button:text-white dark:text-white" />
              </TooltipTrigger>
              <TooltipContent side="right">
                {intl.formatMessage({
                  id: 'common.expand',
                  defaultMessage: 'Expand'
                })}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </SidebarFooter>
      )}
    </React.Fragment>
  )
}
