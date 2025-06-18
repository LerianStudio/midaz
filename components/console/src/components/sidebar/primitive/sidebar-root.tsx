'use client'

import { AnimatePresence, motion } from 'motion/react'
import { useSidebar } from './sidebar-provider'

const sidebarVariants = {
  opened: {
    width: 'auto',
    transition: {
      duration: 0.1
    }
  },
  closed: {
    width: '72px'
  }
}

export const SidebarRoot = ({ children }: React.PropsWithChildren) => {
  const { isCollapsed } = useSidebar()

  return (
    <AnimatePresence>
      <motion.div
        data-collapsed={isCollapsed}
        className="group/sidebar shadow-sidebar dark:bg-cod-gray-950 relative flex flex-col"
        variants={sidebarVariants}
        initial="closed"
        animate={isCollapsed ? 'closed' : 'opened'}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  )
}
