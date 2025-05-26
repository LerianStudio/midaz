import { getServerSession } from 'next-auth'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'

export async function auth() {
  return await getServerSession(nextAuthOptions)
}
