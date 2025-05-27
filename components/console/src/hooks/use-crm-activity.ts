import { useState, useEffect } from 'react'

interface CRMActivity {
  id: string
  type: 'holder_created' | 'holder_updated' | 'alias_created' | 'alias_deleted'
  entityId: string
  entityName: string
  timestamp: Date
  userId?: string
}

export function useCRMActivity(organizationId: string) {
  const [activities, setActivities] = useState<CRMActivity[]>([])
  const storageKey = `crm_activities_${organizationId}`

  useEffect(() => {
    // Load activities from localStorage
    const stored = localStorage.getItem(storageKey)
    if (stored) {
      const parsed = JSON.parse(stored)
      setActivities(parsed.map((a: any) => ({
        ...a,
        timestamp: new Date(a.timestamp)
      })))
    }
  }, [organizationId])

  const logActivity = (activity: Omit<CRMActivity, 'id' | 'timestamp'>) => {
    const newActivity: CRMActivity = {
      ...activity,
      id: crypto.randomUUID(),
      timestamp: new Date()
    }

    setActivities(prev => {
      const updated = [newActivity, ...prev].slice(0, 50) // Keep last 50 activities
      localStorage.setItem(storageKey, JSON.stringify(updated))
      return updated
    })
  }

  const clearActivities = () => {
    setActivities([])
    localStorage.removeItem(storageKey)
  }

  return {
    activities,
    logActivity,
    clearActivities
  }
}