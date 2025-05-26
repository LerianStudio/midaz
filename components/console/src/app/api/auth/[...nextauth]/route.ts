import NextAuth from 'next-auth'

import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'

const handler = NextAuth(nextAuthOptions)

export { handler as GET, handler as POST }
