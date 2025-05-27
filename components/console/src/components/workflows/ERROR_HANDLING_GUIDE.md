# Workflow Error Handling & Loading States Guide

This guide documents the comprehensive error handling and loading state system implemented for workflow components.

## Overview

The error handling system provides:

- **Error Boundaries** - Catch and handle React component errors
- **Loading States** - Consistent loading UI across all workflow components
- **Error Recovery** - Retry mechanisms and user-friendly error messages
- **Error Logging** - Centralized error tracking and monitoring
- **Network Resilience** - Automatic retry with exponential backoff

## Components

### 1. Error Boundary (`error-boundary.tsx`)

A React Error Boundary that catches JavaScript errors in component trees.

```tsx
import { WorkflowErrorBoundaryWrapper } from '@/components/workflows/error-boundary'

export default function MyPage() {
  return (
    <WorkflowErrorBoundaryWrapper
      onError={(error, errorInfo) => {
        // Log to error tracking service
        console.error('Page error:', error, errorInfo)
      }}
    >
      <YourComponent />
    </WorkflowErrorBoundaryWrapper>
  )
}
```

### 2. Loading States (`loading-states.tsx`)

Pre-built loading skeletons for all workflow components:

- `WorkflowPageLoader` - Full page loading
- `WorkflowListSkeleton` - Workflow list table loading
- `WorkflowCanvasSkeleton` - Designer canvas loading
- `ExecutionTimelineSkeleton` - Execution timeline loading
- `ExecutionDetailSkeleton` - Execution detail view loading
- `WorkflowAnalyticsSkeleton` - Analytics dashboard loading
- `TemplateCatalogSkeleton` - Template catalog loading

```tsx
import { WorkflowListSkeleton } from '@/components/workflows/loading-states'

if (isLoading) {
  return <WorkflowListSkeleton />
}
```

### 3. Error Handling Wrapper (`error-handling-wrapper.tsx`)

Combines loading states, error handling, and retry functionality:

```tsx
import { ErrorHandlingWrapper } from '@/components/workflows/error-handling-wrapper'
;<ErrorHandlingWrapper
  isLoading={isLoading}
  error={error}
  onRetry={handleRetry}
  customLoadingComponent={<CustomLoader />}
  customErrorComponent={<CustomError />}
>
  <YourContent />
</ErrorHandlingWrapper>
```

### 4. Enhanced Server Actions (`workflow-server-actions-enhanced.ts`)

Server actions with built-in error classification and retry logic:

```tsx
const result = await createWorkflowActionEnhanced({ workflow })

if (result.success) {
  // Handle success
  console.log(result.data)
} else {
  // Handle typed error
  switch (result.error.type) {
    case 'NETWORK_ERROR':
      // Handle network error
      break
    case 'VALIDATION_ERROR':
      // Handle validation error
      break
    case 'PERMISSION_ERROR':
      // Handle permission error
      break
  }
}
```

## Custom Hooks

### `useAsyncOperation`

Hook for handling async operations with loading and error states:

```tsx
import { useAsyncOperation } from '@/components/workflows/error-handling-wrapper'

function MyComponent() {
  const { isLoading, error, data, execute, retry } =
    useAsyncOperation<WorkflowData>()

  const loadData = () => {
    execute(
      async () => {
        const response = await fetchWorkflowData()
        return response
      },
      {
        onSuccess: (data) => console.log('Loaded:', data),
        onError: (error) => console.error('Failed:', error),
        retryCount: 3,
        retryDelay: 1000
      }
    )
  }

  return (
    <ErrorHandlingWrapper isLoading={isLoading} error={error} onRetry={retry}>
      {/* Your content */}
    </ErrorHandlingWrapper>
  )
}
```

### `useWorkflowData`

Specialized hook for workflow data fetching:

```tsx
import { useWorkflowData } from '@/hooks/use-workflow-data'

function WorkflowEditor({ workflowId }) {
  const { workflow, isLoading, isSaving, error, refetch, update, clearError } =
    useWorkflowData({
      workflowId,
      autoFetch: true,
      onSuccess: (workflow) => console.log('Loaded workflow:', workflow),
      onError: (error) => console.error('Error:', error),
      retryOnError: true
    })

  const handleSave = async () => {
    await update({ name: 'Updated Workflow' })
  }

  return (
    <ErrorHandlingWrapper isLoading={isLoading} error={error} onRetry={refetch}>
      {/* Workflow editor UI */}
    </ErrorHandlingWrapper>
  )
}
```

## Error Types

The system classifies errors into specific types for better handling:

```typescript
enum WorkflowErrorType {
  NETWORK = 'NETWORK', // Connection/fetch errors
  VALIDATION = 'VALIDATION', // Data validation errors
  PERMISSION = 'PERMISSION', // Authorization errors
  NOT_FOUND = 'NOT_FOUND', // Resource not found
  SERVER = 'SERVER', // 5xx server errors
  UNKNOWN = 'UNKNOWN' // Unclassified errors
}
```

## Error Logging

Centralized error logging with `workflow-error-logger.ts`:

```tsx
import { logWorkflowError, useErrorLogger } from '@/lib/workflow-error-logger'

// Direct logging
logWorkflowError(error, {
  component: 'WorkflowDesigner',
  action: 'saveWorkflow'
})

// Using hook
function MyComponent() {
  const { logError, logNetworkError, logValidationError } = useErrorLogger()

  try {
    // Your code
  } catch (error) {
    logError(error, { context: 'additional info' })
  }
}
```

## Best Practices

1. **Always wrap pages in error boundaries**

   ```tsx
   <WorkflowErrorBoundaryWrapper>
     <PageContent />
   </WorkflowErrorBoundaryWrapper>
   ```

2. **Use loading skeletons for better UX**

   ```tsx
   if (isLoading) return <WorkflowListSkeleton />
   ```

3. **Provide retry functionality for network errors**

   ```tsx
   <ErrorHandlingWrapper error={error} onRetry={retry}>
   ```

4. **Log errors with context**

   ```tsx
   logWorkflowError(error, {
     userId,
     workflowId,
     action: 'updateWorkflow'
   })
   ```

5. **Handle different error types appropriately**
   ```tsx
   if (error.type === 'NETWORK_ERROR') {
     // Show connection error UI
   } else if (error.type === 'VALIDATION_ERROR') {
     // Show validation errors
   }
   ```

## Example Implementation

Here's a complete example of a workflow page with comprehensive error handling:

```tsx
'use client'

import { WorkflowErrorBoundaryWrapper } from '@/components/workflows/error-boundary'
import { ErrorHandlingWrapper } from '@/components/workflows/error-handling-wrapper'
import { WorkflowListSkeleton } from '@/components/workflows/loading-states'
import { useWorkflowData } from '@/hooks/use-workflow-data'
import { useErrorLogger } from '@/lib/workflow-error-logger'

function WorkflowListContent() {
  const { logError } = useErrorLogger()
  const { workflows, isLoading, error, refetch } = useWorkflowData({
    autoFetch: true,
    onError: (error) => {
      logError(error, { page: 'WorkflowList' })
    }
  })

  return (
    <ErrorHandlingWrapper
      isLoading={isLoading}
      error={error}
      onRetry={refetch}
      customLoadingComponent={<WorkflowListSkeleton />}
    >
      {/* Your workflow list UI */}
    </ErrorHandlingWrapper>
  )
}

export default function WorkflowListPage() {
  return (
    <WorkflowErrorBoundaryWrapper>
      <WorkflowListContent />
    </WorkflowErrorBoundaryWrapper>
  )
}
```

## Testing Error Handling

To test error scenarios:

1. **Network errors**: Disconnect network or use browser dev tools
2. **Server errors**: Mock server to return 500 errors
3. **Validation errors**: Submit invalid data
4. **Component errors**: Throw errors in component code
5. **Permission errors**: Test with restricted user roles

## Monitoring & Analytics

The error logging system can integrate with monitoring services:

- Sentry for error tracking
- Google Analytics for error metrics
- Custom logging endpoints
- Browser console in development

Configure via environment variables:

```env
NEXT_PUBLIC_ERROR_LOGGING_ENDPOINT=https://your-logging-service.com/api/errors
```
