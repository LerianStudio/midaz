'use client'

import { createContext, useContext, useEffect, useRef, ReactNode } from 'react'
import { io, Socket } from 'socket.io-client'
import { useRealtimeStore } from '@/store'
import { useToast } from '@/hooks/use-toast'

interface WebSocketContextType {
  socket: Socket | null
  subscribe: (event: string, callback: (data: any) => void) => void
  unsubscribe: (event: string, callback?: (data: any) => void) => void
  emit: (event: string, data: any) => void
}

const WebSocketContext = createContext<WebSocketContextType>({
  socket: null,
  subscribe: () => {},
  unsubscribe: () => {},
  emit: () => {}
})

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const socketRef = useRef<Socket | null>(null)
  const { toast } = useToast()
  const {
    setConnected,
    setConnectionStatus,
    updateHeartbeat,
    subscribedChannels,
    addPendingUpdate
  } = useRealtimeStore()

  // Event handlers registry
  const eventHandlers = useRef<Map<string, Set<(data: any) => void>>>(new Map())

  const subscribe = (event: string, callback: (data: any) => void) => {
    if (!eventHandlers.current.has(event)) {
      eventHandlers.current.set(event, new Set())
    }
    eventHandlers.current.get(event)?.add(callback)

    // If socket is connected, subscribe immediately
    if (socketRef.current?.connected) {
      socketRef.current.on(event, callback)
    }
  }

  const unsubscribe = (event: string, callback?: (data: any) => void) => {
    if (callback) {
      eventHandlers.current.get(event)?.delete(callback)
      socketRef.current?.off(event, callback)
    } else {
      eventHandlers.current.delete(event)
      socketRef.current?.off(event)
    }
  }

  const emit = (event: string, data: any) => {
    if (socketRef.current?.connected) {
      socketRef.current.emit(event, data)
    } else {
      // Queue the event for when connection is restored
      addPendingUpdate(event, data)
    }
  }

  useEffect(() => {
    const WEBSOCKET_URL =
      process.env.NEXT_PUBLIC_WEBSOCKET_URL || 'http://localhost:3002'

    // Initialize socket connection
    socketRef.current = io(WEBSOCKET_URL, {
      transports: ['websocket'],
      reconnection: true,
      reconnectionDelay: 1000,
      reconnectionDelayMax: 5000,
      reconnectionAttempts: 5
    })

    const socket = socketRef.current

    // Connection event handlers
    socket.on('connect', () => {
      setConnected(true)
      console.log('🔌 WebSocket connected')

      // Re-subscribe to all channels
      subscribedChannels.forEach((channel) => {
        socket.emit('subscribe', { channel })
      })

      // Re-attach all event handlers
      eventHandlers.current.forEach((handlers, event) => {
        handlers.forEach((handler) => {
          socket.on(event, handler)
        })
      })
    })

    socket.on('disconnect', (reason: string) => {
      setConnected(false)
      console.log('🔌 WebSocket disconnected:', reason)

      if (reason === 'io server disconnect') {
        // Server initiated disconnect, attempt to reconnect
        socket.connect()
      }
    })

    socket.on('connect_error', (error: Error) => {
      setConnectionStatus('error')
      console.error('🔌 WebSocket connection error:', error)

      toast({
        title: 'Connection Error',
        description: 'Unable to establish real-time connection',
        variant: 'destructive'
      })
    })

    socket.on('reconnect', (attemptNumber: number) => {
      console.log('🔌 WebSocket reconnected after', attemptNumber, 'attempts')
      toast({
        title: 'Connection Restored',
        description: 'Real-time updates are now active'
      })
    })

    socket.on('reconnect_attempt', (attemptNumber: number) => {
      setConnectionStatus('connecting')
      console.log('🔌 WebSocket reconnection attempt', attemptNumber)
    })

    socket.on('reconnect_failed', () => {
      setConnectionStatus('error')
      toast({
        title: 'Connection Failed',
        description: 'Unable to restore real-time connection',
        variant: 'destructive'
      })
    })

    // Heartbeat mechanism
    const heartbeatInterval = setInterval(() => {
      if (socket.connected) {
        socket.emit('heartbeat')
        updateHeartbeat()
      }
    }, 30000) // 30 seconds

    // Cleanup
    return () => {
      clearInterval(heartbeatInterval)
      socket.disconnect()
      socketRef.current = null
    }
  }, [])

  return (
    <WebSocketContext.Provider
      value={{
        socket: socketRef.current,
        subscribe,
        unsubscribe,
        emit
      }}
    >
      {children}
    </WebSocketContext.Provider>
  )
}

export const useWebSocket = () => {
  const context = useContext(WebSocketContext)
  if (!context) {
    throw new Error('useWebSocket must be used within WebSocketProvider')
  }
  return context
}
