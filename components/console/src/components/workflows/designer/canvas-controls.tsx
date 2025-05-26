'use client'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import {
  Settings,
  CheckCircle,
  Save,
  Play,
  Eye,
  Download,
  Upload,
  Undo,
  Redo,
  ZoomIn,
  ZoomOut,
  Maximize,
  Grid
} from 'lucide-react'

interface CanvasControlsProps {
  onMetadataClick: () => void
  onValidate: () => void
  onSave: () => void
  onPreview?: () => void
  onExport?: () => void
  onImport?: () => void
  onUndo?: () => void
  onRedo?: () => void
  onZoomIn?: () => void
  onZoomOut?: () => void
  onFitView?: () => void
  onToggleGrid?: () => void
  readonly?: boolean
  hasUnsavedChanges?: boolean
  isValid?: boolean
}

export function CanvasControls({
  onMetadataClick,
  onValidate,
  onSave,
  onPreview,
  onExport,
  onImport,
  onUndo,
  onRedo,
  onZoomIn,
  onZoomOut,
  onFitView,
  onToggleGrid,
  readonly = false,
  hasUnsavedChanges = false,
  isValid = true
}: CanvasControlsProps) {
  return (
    <TooltipProvider>
      <Card className="p-2">
        <div className="flex items-center space-x-1">
          {/* Primary Actions */}
          <div className="mr-2 flex items-center space-x-1">
            {!readonly && (
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={onMetadataClick}
                      className="h-8 w-8 p-0"
                    >
                      <Settings className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Workflow Settings</p>
                  </TooltipContent>
                </Tooltip>

                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={onValidate}
                      className="h-8 w-8 p-0"
                    >
                      <CheckCircle
                        className={`h-4 w-4 ${isValid ? 'text-green-600' : 'text-red-600'}`}
                      />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Validate Workflow</p>
                  </TooltipContent>
                </Tooltip>

                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={onSave}
                      className="relative h-8 w-8 p-0"
                    >
                      <Save className="h-4 w-4" />
                      {hasUnsavedChanges && (
                        <span className="absolute -right-1 -top-1 h-2 w-2 rounded-full bg-orange-500"></span>
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Save Workflow</p>
                  </TooltipContent>
                </Tooltip>
              </>
            )}

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onPreview}
                  className="h-8 w-8 p-0"
                >
                  <Eye className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Preview Workflow</p>
              </TooltipContent>
            </Tooltip>
          </div>

          {/* Divider */}
          <div className="mx-1 h-6 w-px bg-border"></div>

          {/* Edit Actions */}
          {!readonly && (
            <div className="mr-2 flex items-center space-x-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onUndo}
                    className="h-8 w-8 p-0"
                  >
                    <Undo className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Undo</p>
                </TooltipContent>
              </Tooltip>

              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onRedo}
                    className="h-8 w-8 p-0"
                  >
                    <Redo className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Redo</p>
                </TooltipContent>
              </Tooltip>
            </div>
          )}

          {/* Divider */}
          <div className="mx-1 h-6 w-px bg-border"></div>

          {/* View Controls */}
          <div className="mr-2 flex items-center space-x-1">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onZoomIn}
                  className="h-8 w-8 p-0"
                >
                  <ZoomIn className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Zoom In</p>
              </TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onZoomOut}
                  className="h-8 w-8 p-0"
                >
                  <ZoomOut className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Zoom Out</p>
              </TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onFitView}
                  className="h-8 w-8 p-0"
                >
                  <Maximize className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Fit to View</p>
              </TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onToggleGrid}
                  className="h-8 w-8 p-0"
                >
                  <Grid className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Toggle Grid</p>
              </TooltipContent>
            </Tooltip>
          </div>

          {/* Divider */}
          <div className="mx-1 h-6 w-px bg-border"></div>

          {/* Import/Export */}
          <div className="flex items-center space-x-1">
            {!readonly && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onImport}
                    className="h-8 w-8 p-0"
                  >
                    <Upload className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Import Workflow</p>
                </TooltipContent>
              </Tooltip>
            )}

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onExport}
                  className="h-8 w-8 p-0"
                >
                  <Download className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Export Workflow</p>
              </TooltipContent>
            </Tooltip>
          </div>

          {/* Status Indicators */}
          <div className="ml-4 flex items-center space-x-2">
            {hasUnsavedChanges && (
              <Badge variant="outline" className="text-xs">
                Unsaved Changes
              </Badge>
            )}

            <Badge
              variant={isValid ? 'default' : 'destructive'}
              className="text-xs"
            >
              {isValid ? 'Valid' : 'Invalid'}
            </Badge>

            {readonly && (
              <Badge variant="secondary" className="text-xs">
                Read Only
              </Badge>
            )}
          </div>
        </div>
      </Card>
    </TooltipProvider>
  )
}
