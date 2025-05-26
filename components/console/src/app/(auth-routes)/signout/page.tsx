'use client'

import { useEffect } from 'react'
import { signOut } from 'next-auth/react'

export default function Page() {
  useEffect(() => {
    signOut({ callbackUrl: '/' })
  }, [])

  return null
}
