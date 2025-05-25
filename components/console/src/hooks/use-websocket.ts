'use client'

import { useEffect, useRef, useState, useCallback } from 'react'
import {
  getWebSocketClient,
  WebSocketMessage,
  ExecutionUpdateMessage
} from '@/core/infrastructure/websocket/websocket-client'
import { toast } from '@/hooks/use-toast'

interface UseWebSocketOptions {
  autoConnect?: boolean
  onConnect?: () => void
  onDisconnect?: () => void
  onError?: (error: any) => void
}

export function useWebSocket(options: UseWebSocketOptions = {}) {
  const { autoConnect = true, onConnect, onDisconnect, onError } = options
  const [isConnected, setIsConnected] = useState(false)
  const [isConnecting, setIsConnecting] = useState(false)
  const wsClient = useRef(getWebSocketClient())

  useEffect(() => {
    const client = wsClient.current

    // Subscribe to connection events
    const unsubConnect = client.subscribe('connection', (message) => {
      setIsConnected(message.data.connected)
      setIsConnecting(false)
      if (message.data.connected) {
        onConnect?.()
      } else {
        onDisconnect?.()
      }
    })

    // Subscribe to error events
    const unsubError = client.subscribe('error', (message) => {
      console.error('WebSocket error:', message.data)
      setIsConnecting(false)
      onError?.(message.data.error)
      toast({
        variant: 'destructive',
        title: 'Connection Error',
        description:
          'Failed to establish real-time connection. Updates may be delayed.'
      })
    })

    // Auto-connect if enabled
    if (autoConnect && !client.isConnected()) {
      setIsConnecting(true)
      client.connect().catch((error) => {
        console.error('Failed to connect WebSocket:', error)
        setIsConnecting(false)
      })
    }

    return () => {
      unsubConnect()
      unsubError()
    }
  }, [autoConnect, onConnect, onDisconnect, onError])

  const connect = useCallback(async () => {
    if (!wsClient.current.isConnected() && !isConnecting) {
      setIsConnecting(true)
      try {
        await wsClient.current.connect()
      } catch (error) {
        console.error('Failed to connect:', error)
        setIsConnecting(false)
      }
    }
  }, [isConnecting])

  const disconnect = useCallback(() => {
    wsClient.current.disconnect()
  }, [])

  const subscribe = useCallback(
    (event: string, callback: (message: WebSocketMessage) => void) => {
      return wsClient.current.subscribe(event, callback)
    },
    []
  )

  const send = useCallback((message: any) => {
    wsClient.current.send(message)
  }, [])

  return {
    isConnected,
    isConnecting,
    connect,
    disconnect,
    subscribe,
    send,
    client: wsClient.current
  }
}

// Hook for subscribing to execution updates
export function useExecutionUpdates(
  executionId: string | null,
  onUpdate?: (update: ExecutionUpdateMessage) => void
) {
  const { client, isConnected } = useWebSocket()
  const [lastUpdate, setLastUpdate] = useState<ExecutionUpdateMessage | null>(
    null
  )

  useEffect(() => {
    if (!executionId || !isConnected) return

    const unsubscribe = client.subscribeToExecution(executionId, (update) => {
      setLastUpdate(update)
      onUpdate?.(update)
    })

    return unsubscribe
  }, [executionId, isConnected, client, onUpdate])

  return { lastUpdate, isConnected }
}

// Hook for subscribing to workflow updates
export function useWorkflowUpdates(
  workflowId: string | null,
  onUpdate?: (update: any) => void
) {
  const { client, isConnected } = useWebSocket()
  const [lastUpdate, setLastUpdate] = useState<any>(null)

  useEffect(() => {
    if (!workflowId || !isConnected) return

    const unsubscribe = client.subscribeToWorkflow(workflowId, (update) => {
      setLastUpdate(update)
      onUpdate?.(update)
    })

    return unsubscribe
  }, [workflowId, isConnected, client, onUpdate])

  return { lastUpdate, isConnected }
}

// Hook for monitoring multiple executions
export function useExecutionMonitoring(executionIds: string[]) {
  const { client, isConnected } = useWebSocket()
  const [updates, setUpdates] = useState<Map<string, ExecutionUpdateMessage>>(
    new Map()
  )

  useEffect(() => {
    if (executionIds.length === 0 || !isConnected) return

    const unsubscribes = executionIds.map((id) =>
      client.subscribeToExecution(id, (update) => {
        setUpdates((prev) => new Map(prev).set(id, update))
      })
    )

    return () => {
      unsubscribes.forEach((unsub) => unsub())
    }
  }, [executionIds, isConnected, client])

  return { updates, isConnected }
}
