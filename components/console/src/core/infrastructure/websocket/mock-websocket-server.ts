// Mock WebSocket server for development
// This simulates a WebSocket server that sends updates for workflow executions

import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'

export class MockWebSocketServer {
  private connections: Set<MockWebSocketConnection> = new Set()
  private executionSimulations: Map<string, NodeJS.Timeout> = new Map()

  constructor() {
    // Start simulating updates for running executions
    this.startSimulations()
  }

  connect(): MockWebSocketConnection {
    const connection = new MockWebSocketConnection(this)
    this.connections.add(connection)
    return connection
  }

  disconnect(connection: MockWebSocketConnection): void {
    this.connections.delete(connection)
  }

  broadcast(message: any): void {
    this.connections.forEach((conn) => conn.receive(message))
  }

  broadcastToSubscribers(event: string, message: any): void {
    this.connections.forEach((conn) => {
      if (conn.isSubscribed(event)) {
        conn.receive(message)
      }
    })
  }

  private startSimulations(): void {
    // Find running executions and simulate updates
    const runningExecutions = mockWorkflowExecutions.filter(
      (e) => e.status === 'RUNNING'
    )

    runningExecutions.forEach((execution) => {
      this.simulateExecutionProgress(execution.executionId)
    })
  }

  private simulateExecutionProgress(executionId: string): void {
    let progress = 0

    const interval = setInterval(
      () => {
        progress += Math.random() * 20

        if (progress >= 100) {
          // Complete the execution
          this.broadcastToSubscribers(`execution:${executionId}`, {
            type: 'execution_update',
            data: {
              executionId,
              status: 'COMPLETED',
              updates: {
                status: 'COMPLETED',
                endTime: new Date(),
                output: {
                  result: 'success',
                  processedItems: Math.floor(Math.random() * 1000)
                }
              }
            },
            timestamp: Date.now()
          })

          clearInterval(interval)
          this.executionSimulations.delete(executionId)
        } else {
          // Send progress update
          this.broadcastToSubscribers(`execution:${executionId}`, {
            type: 'task_update',
            data: {
              executionId,
              taskId: `task_${Date.now()}`,
              updates: {
                progress,
                status: 'IN_PROGRESS',
                message: `Processing... ${Math.floor(progress)}%`
              }
            },
            timestamp: Date.now()
          })
        }
      },
      2000 + Math.random() * 3000
    ) // Random interval between 2-5 seconds

    this.executionSimulations.set(executionId, interval)
  }

  startExecution(executionId: string): void {
    // Start simulating a new execution
    this.simulateExecutionProgress(executionId)
  }

  stopSimulation(executionId: string): void {
    const interval = this.executionSimulations.get(executionId)
    if (interval) {
      clearInterval(interval)
      this.executionSimulations.delete(executionId)
    }
  }

  cleanup(): void {
    // Clear all simulations
    this.executionSimulations.forEach((interval) => clearInterval(interval))
    this.executionSimulations.clear()
    this.connections.clear()
  }
}

export class MockWebSocketConnection {
  private server: MockWebSocketServer
  private subscriptions: Set<string> = new Set()
  private messageQueue: any[] = []
  private onMessageCallback: ((data: string) => void) | null = null
  private onOpenCallback: (() => void) | null = null
  private onCloseCallback: (() => void) | null = null
  private onErrorCallback: ((error: any) => void) | null = null
  public readyState: number = WebSocket.CONNECTING

  constructor(server: MockWebSocketServer) {
    this.server = server

    // Simulate connection delay
    setTimeout(() => {
      this.readyState = WebSocket.OPEN
      this.onOpenCallback?.()

      // Send any queued messages
      this.messageQueue.forEach((msg) => this.receive(msg))
      this.messageQueue = []
    }, 100)
  }

  set onopen(callback: () => void) {
    this.onOpenCallback = callback
    if (this.readyState === WebSocket.OPEN) {
      callback()
    }
  }

  set onmessage(callback: (event: { data: string }) => void) {
    this.onMessageCallback = callback
  }

  set onclose(callback: () => void) {
    this.onCloseCallback = callback
  }

  set onerror(callback: (error: any) => void) {
    this.onErrorCallback = callback
  }

  send(data: string): void {
    if (this.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not open')
    }

    // Parse the message and handle subscriptions
    try {
      const message = JSON.parse(data)

      if (message.type === 'subscribe_execution') {
        this.subscriptions.add(`execution:${message.executionId}`)
        // Send initial data
        setTimeout(() => {
          this.receive({
            type: 'execution_update',
            data: {
              executionId: message.executionId,
              status: 'RUNNING',
              updates: {}
            },
            timestamp: Date.now()
          })
        }, 100)
      } else if (message.type === 'subscribe_workflow') {
        this.subscriptions.add(`workflow:${message.workflowId}`)
      } else if (message.type === 'ping') {
        // Respond with pong
        this.receive({ type: 'pong', timestamp: Date.now() })
      }
    } catch (error) {
      console.error('Failed to parse WebSocket message:', error)
    }
  }

  receive(message: any): void {
    if (this.readyState !== WebSocket.OPEN) {
      this.messageQueue.push(message)
      return
    }

    if (this.onMessageCallback) {
      this.onMessageCallback({ data: JSON.stringify(message) })
    }
  }

  close(): void {
    this.readyState = WebSocket.CLOSED
    this.server.disconnect(this)
    this.onCloseCallback?.()
  }

  isSubscribed(event: string): boolean {
    return this.subscriptions.has(event)
  }
}

// Global mock server instance
let mockServer: MockWebSocketServer | null = null

export function getMockWebSocketServer(): MockWebSocketServer {
  if (!mockServer) {
    mockServer = new MockWebSocketServer()
  }
  return mockServer
}

// Override WebSocket in development
if (process.env.NODE_ENV === 'development' && typeof window !== 'undefined') {
  const OriginalWebSocket = window.WebSocket

  ;(window as any).WebSocket = class MockWebSocket {
    private connection: MockWebSocketConnection

    constructor(url: string) {
      console.log('Mock WebSocket connection to:', url)
      this.connection = getMockWebSocketServer().connect()

      // Proxy all properties and methods
      Object.defineProperty(this, 'readyState', {
        get: () => this.connection.readyState
      })
    }

    set onopen(callback: () => void) {
      this.connection.onopen = callback
    }

    set onmessage(callback: (event: { data: string }) => void) {
      this.connection.onmessage = callback
    }

    set onclose(callback: () => void) {
      this.connection.onclose = callback
    }

    set onerror(callback: (error: any) => void) {
      this.connection.onerror = callback
    }

    send(data: string): void {
      this.connection.send(data)
    }

    close(): void {
      this.connection.close()
    }

    // WebSocket constants
    static CONNECTING = 0
    static OPEN = 1
    static CLOSING = 2
    static CLOSED = 3
  }
}
