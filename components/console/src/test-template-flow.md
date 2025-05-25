# Template Instantiation Flow Test Plan

## Overview

This document outlines the testing steps for the workflow template instantiation feature.

## Test Scenarios

### 1. Navigate from Template Catalog

1. Go to `/plugins/workflows/library`
2. Click on "Templates" tab
3. Find a template and click "Use" button
4. Should navigate to `/plugins/workflows/library/create?templateId={id}`
5. Verify the creation wizard is pre-filled with template data

### 2. Direct URL with Template ID

1. Navigate directly to `/plugins/workflows/library/create?templateId=template-1`
2. Verify the wizard loads with:
   - Pre-filled name (template name + date)
   - Pre-filled description
   - Pre-filled category
   - Pre-filled tags
   - Starting on step 3 (Parameters)
   - Template selected in step 2

### 3. Create Workflow from Template

1. From the pre-filled wizard, complete the workflow creation
2. Click "Create Workflow"
3. Should navigate to `/plugins/workflows/library/new/designer`
4. Verify the designer loads with:
   - Template tasks pre-loaded
   - Input/output parameters from template
   - Metadata including templateId

### 4. Template with Parameters

1. When using template instantiation dialog:
   - Fill in required parameters
   - Click "Create Workflow"
   - Should navigate with both templateId and parameters in URL
2. Verify parameters are passed through to the workflow

## Implementation Details

### URL Parameters

- `templateId`: The ID of the template to use
- `parameters`: URL-encoded JSON object of template parameters

### Session Storage

- Template data is temporarily stored in session storage
- Key format: `workflow-template-{workflowId}`
- Cleaned up after designer loads the data

### Components Updated

1. **WorkflowCreationWizard**:

   - Accepts `initialTemplate` and `initialTemplateParameters` props
   - Pre-fills form when template is provided
   - Starts on appropriate step

2. **CreateWorkflowPage**:

   - Reads templateId and parameters from URL
   - Loads template from mock data
   - Passes to wizard component

3. **TemplateCatalog**:

   - Navigates to creation page with templateId
   - Handles both embedded (with callback) and standalone modes

4. **WorkflowDesignerPage**:
   - Loads template data from session storage for new workflows
   - Initializes workflow with template tasks and parameters
