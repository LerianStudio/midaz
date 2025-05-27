import { injectable } from 'inversify'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'

export interface WebSocketMessage {
  type:
    | 'execution_update'
    | 'task_update'
    | 'workflow_update'
    | 'connection'
    | 'error'
  data: any
  timestamp: number
}

export interface ExecutionUpdateMessage {
  executionId: string
  status: WorkflowExecution['status']
  updates: Partial<WorkflowExecution>
}

export interface TaskUpdateMessage {
  executionId: string
  taskId: string
  updates: any
}

export type WebSocketCallback = (message: WebSocketMessage) => void

@injectable()
export class WebSocketClient {
  private ws: WebSocket | null = null
  private url: string
  private reconnectInterval: number = 5000
  private maxReconnectAttempts: number = 5
  private reconnectAttempts: number = 0
  private callbacks: Map<string, WebSocketCallback[]> = new Map()
  private isConnecting: boolean = false
  private heartbeatInterval: NodeJS.Timeout | null = null

  constructor() {
    // In production, this would come from environment config
    this.url = process.env.NEXT_PUBLIC_WEBSOCKET_URL || ''
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      // For demo purposes, skip WebSocket connection
      if (!this.url || this.url === '' || this.url.includes('localhost')) {
        console.log('Demo mode: Skipping WebSocket connection')
        // Don't emit connection event to avoid triggering error handling
        resolve()
        return
      }

      if (this.ws?.readyState === WebSocket.OPEN) {
        resolve()
        return
      }

      if (this.isConnecting) {
        // Wait for existing connection attempt
        const checkConnection = setInterval(() => {
          if (this.ws?.readyState === WebSocket.OPEN) {
            clearInterval(checkConnection)
            resolve()
          }
        }, 100)
        return
      }

      this.isConnecting = true

      try {
        this.ws = new WebSocket(this.url)

        this.ws.onopen = () => {
          console.log('WebSocket connected')
          this.isConnecting = false
          this.reconnectAttempts = 0
          this.startHeartbeat()
          this.emit('connection', { connected: true })
          resolve()
        }

        this.ws.onmessage = (event) => {
          try {
            const message: WebSocketMessage = JSON.parse(event.data)
            this.handleMessage(message)
          } catch (error) {
            console.error('Failed to parse WebSocket message:', error)
          }
        }

        this.ws.onerror = (error) => {
          console.error('WebSocket error:', error)
          this.isConnecting = false
          // Only emit error for non-demo environments
          if (this.url && !this.url.includes('localhost')) {
            this.emit('error', { error })
          }
          reject(error)
        }

        this.ws.onclose = () => {
          console.log('WebSocket disconnected')
          this.isConnecting = false
          this.stopHeartbeat()
          this.emit('connection', { connected: false })
          this.attemptReconnect()
        }
      } catch (error) {
        this.isConnecting = false
        reject(error)
      }
    })
  }

  disconnect(): void {
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.stopHeartbeat()
    this.callbacks.clear()
  }

  subscribe(event: string, callback: WebSocketCallback): () => void {
    if (!this.callbacks.has(event)) {
      this.callbacks.set(event, [])
    }
    this.callbacks.get(event)!.push(callback)

    // Return unsubscribe function
    return () => {
      const callbacks = this.callbacks.get(event)
      if (callbacks) {
        const index = callbacks.indexOf(callback)
        if (index > -1) {
          callbacks.splice(index, 1)
        }
      }
    }
  }

  send(message: any): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(message))
    } else {
      console.warn('WebSocket is not connected')
    }
  }

  // Subscribe to specific execution updates
  subscribeToExecution(
    executionId: string,
    callback: (update: ExecutionUpdateMessage) => void
  ): () => void {
    // Send subscription message to server
    this.send({
      type: 'subscribe_execution',
      executionId
    })

    // Subscribe to updates
    return this.subscribe(`execution:${executionId}`, (message) => {
      if (
        message.type === 'execution_update' &&
        message.data.executionId === executionId
      ) {
        callback(message.data)
      }
    })
  }

  // Subscribe to workflow updates
  subscribeToWorkflow(
    workflowId: string,
    callback: (update: any) => void
  ): () => void {
    this.send({
      type: 'subscribe_workflow',
      workflowId
    })

    return this.subscribe(`workflow:${workflowId}`, (message) => {
      if (
        message.type === 'workflow_update' &&
        message.data.workflowId === workflowId
      ) {
        callback(message.data)
      }
    })
  }

  private handleMessage(message: WebSocketMessage): void {
    // Emit to general listeners
    this.emit(message.type, message.data)

    // Emit to specific listeners
    if (message.type === 'execution_update' && message.data.executionId) {
      this.emit(`execution:${message.data.executionId}`, message)
    } else if (message.type === 'workflow_update' && message.data.workflowId) {
      this.emit(`workflow:${message.data.workflowId}`, message)
    } else if (message.type === 'task_update' && message.data.executionId) {
      this.emit(`task:${message.data.executionId}`, message)
    }
  }

  private emit(event: string, data: any): void {
    const callbacks = this.callbacks.get(event)
    if (callbacks) {
      callbacks.forEach((callback) => {
        try {
          callback({
            type: event as any,
            data,
            timestamp: Date.now()
          })
        } catch (error) {
          console.error('Error in WebSocket callback:', error)
        }
      })
    }
  }

  private attemptReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnection attempts reached')
      return
    }

    this.reconnectAttempts++
    console.log(
      `Attempting to reconnect (${this.reconnectAttempts}/${this.maxReconnectAttempts})...`
    )

    setTimeout(() => {
      this.connect().catch((error) => {
        console.error('Reconnection failed:', error)
      })
    }, this.reconnectInterval)
  }

  private startHeartbeat(): void {
    this.heartbeatInterval = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.send({ type: 'ping' })
      }
    }, 30000) // Send ping every 30 seconds
  }

  private stopHeartbeat(): void {
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval)
      this.heartbeatInterval = null
    }
  }

  isConnected(): boolean {
    // For demo mode, always return false without trying to connect
    if (!this.url || this.url === '' || this.url.includes('localhost')) {
      return false
    }
    return this.ws?.readyState === WebSocket.OPEN
  }
}

// Singleton instance
let wsClient: WebSocketClient | null = null

export function getWebSocketClient(): WebSocketClient {
  if (!wsClient) {
    wsClient = new WebSocketClient()
  }
  return wsClient
}
