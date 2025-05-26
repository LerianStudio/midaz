'use client'

import { AnimatePresence, motion } from 'framer-motion'
import { useSidebar } from './sidebar-provider'

const sidebarVariants = {
  opened: {
    width: 'auto',
    transition: {
      duration: 0.1,
      ease: 'easeInOut'
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
        className="group/sidebar relative flex flex-col shadow-sidebar dark:bg-codGray-950"
        variants={sidebarVariants}
        initial="closed"
        animate={isCollapsed ? 'closed' : 'opened'}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  )
}
