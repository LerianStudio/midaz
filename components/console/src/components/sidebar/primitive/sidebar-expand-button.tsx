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
        <div className="flex w-full border-shadcn-200 bg-white">
          <div className="absolute bottom-4 right-[-20px]">
            <Button
              variant="white"
              className="rounded-full border border-shadcn-200 p-2"
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
                className="group/expand-button rounded-sm bg-transparent p-2 text-shadcn-400 hover:bg-accent"
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
