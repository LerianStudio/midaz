import { RealTimeMonitoringDashboard } from '@/components/workflows/executions/real-time-monitoring-dashboard'

export default function MonitoringPage() {
  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-2xl font-bold">Workflow Monitoring</h1>
        <p className="text-muted-foreground">
          Real-time monitoring and system health overview
        </p>
      </div>

      <RealTimeMonitoringDashboard />
    </div>
  )
}
