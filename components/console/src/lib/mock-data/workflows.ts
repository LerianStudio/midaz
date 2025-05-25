import { Workflow, WorkflowStatus } from '@/core/domain/entities/workflow'
import {
  WorkflowExecution,
  ExecutionStatus
} from '@/core/domain/entities/workflow-execution'
import {
  WorkflowTemplate,
  TemplateCategory
} from '@/core/domain/entities/workflow-template'

// Mock Workflows Data
export const mockWorkflows: Workflow[] = [
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231c',
    name: 'payment_processing_flow',
    description: 'Complete payment processing with fees and validation',
    version: 2,
    status: 'ACTIVE',
    tasks: [
      {
        name: 'validate_accounts',
        taskReferenceName: 'account_validation',
        type: 'HTTP',
        description: 'Validate source and destination accounts',
        inputParameters: {
          http_request: {
            uri: 'http://midaz-onboarding:3000/v1/accounts/${workflow.input.sourceAccount}',
            method: 'GET',
            headers: {
              'Content-Type': 'application/json',
              Authorization: 'Bearer ${workflow.input.token}'
            }
          }
        },
        retryCount: 3,
        timeoutSeconds: 30
      },
      {
        name: 'calculate_fees',
        taskReferenceName: 'fee_calculation',
        type: 'HTTP',
        description: 'Calculate applicable fees for the transaction',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-fees:4002/v1/fees/calculate',
            method: 'POST',
            headers: {
              'Content-Type': 'application/json'
            },
            body: {
              amount: '${workflow.input.amount}',
              transactionType: 'transfer',
              sourceAccount: '${workflow.input.sourceAccount}',
              destinationAccount: '${workflow.input.destinationAccount}'
            }
          }
        },
        retryCount: 2,
        timeoutSeconds: 20
      },
      {
        name: 'fee_decision',
        taskReferenceName: 'fee_switch',
        type: 'SWITCH',
        description: 'Decision based on fee calculation results',
        inputParameters: {
          switchCaseValue: '${fee_calculation.output.response.body.status}',
          decisionCases: {
            approved: [
              {
                name: 'create_transaction',
                taskReferenceName: 'transaction_creation',
                type: 'HTTP'
              }
            ],
            requires_approval: [
              {
                name: 'request_approval',
                taskReferenceName: 'approval_request',
                type: 'HUMAN'
              }
            ],
            rejected: [
              {
                name: 'send_rejection_notice',
                taskReferenceName: 'rejection_notice',
                type: 'HTTP'
              }
            ]
          }
        }
      },
      {
        name: 'create_transaction',
        taskReferenceName: 'transaction_creation',
        type: 'HTTP',
        description: 'Create the actual transaction',
        inputParameters: {
          http_request: {
            uri: 'http://midaz-transaction:3001/v1/transactions',
            method: 'POST',
            headers: {
              'Content-Type': 'application/json'
            },
            body: {
              send: {
                source: {
                  from: '${workflow.input.sourceAccount}',
                  amount: '${workflow.input.amount}',
                  asset: '${workflow.input.currency}'
                },
                destination: {
                  to: '${workflow.input.destinationAccount}',
                  amount: '${workflow.input.amount}',
                  asset: '${workflow.input.currency}'
                }
              }
            }
          }
        },
        retryCount: 1,
        timeoutSeconds: 45
      },
      {
        name: 'send_confirmation',
        taskReferenceName: 'confirmation_notification',
        type: 'HTTP',
        description: 'Send transaction confirmation notification',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-crm:4003/v1/notifications/send',
            method: 'POST',
            body: {
              recipient: '${workflow.input.customerEmail}',
              type: 'transaction_confirmation',
              data: {
                transactionId:
                  '${transaction_creation.output.response.body.id}',
                amount: '${workflow.input.amount}',
                currency: '${workflow.input.currency}'
              }
            }
          }
        },
        optional: true
      }
    ],
    inputParameters: [
      'sourceAccount',
      'destinationAccount',
      'amount',
      'currency',
      'customerEmail',
      'token'
    ],
    outputParameters: ['transactionId', 'status', 'fees'],
    createdBy: 'admin@company.com',
    executionCount: 1247,
    lastExecuted: '2025-01-01T14:30:00Z',
    avgExecutionTime: '2.3s',
    successRate: 0.987,
    createdAt: '2024-11-15T00:00:00Z',
    updatedAt: '2024-12-20T00:00:00Z',
    metadata: {
      category: 'payments',
      tags: ['payment', 'transaction', 'fees', 'validation'],
      author: 'Payment Team',
      schemaVersion: '1.0',
      timeoutPolicy: {
        timeoutSeconds: 300,
        alertAfterTimeoutSeconds: 240
      },
      restartable: true,
      ownerEmail: 'payments@company.com'
    }
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231d',
    name: 'customer_onboarding_flow',
    description: 'Complete customer onboarding with KYC and account creation',
    version: 1,
    status: 'ACTIVE',
    tasks: [
      {
        name: 'kyc_verification',
        taskReferenceName: 'kyc_check',
        type: 'HTTP',
        description: 'Perform KYC verification through identity service',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-identity:4001/v1/verify',
            method: 'POST',
            headers: {
              'Content-Type': 'application/json'
            },
            body: '${workflow.input.customerData}'
          }
        },
        retryCount: 2,
        timeoutSeconds: 60
      },
      {
        name: 'kyc_decision',
        taskReferenceName: 'kyc_switch',
        type: 'SWITCH',
        description: 'Decision based on KYC verification results',
        inputParameters: {
          switchCaseValue: '${kyc_check.output.response.body.status}',
          decisionCases: {
            approved: [
              {
                name: 'create_customer_record',
                taskReferenceName: 'create_customer',
                type: 'HTTP'
              }
            ],
            pending: [
              {
                name: 'request_manual_review',
                taskReferenceName: 'manual_review',
                type: 'HUMAN'
              }
            ],
            rejected: [
              {
                name: 'send_rejection_notice',
                taskReferenceName: 'rejection_notice',
                type: 'HTTP'
              }
            ]
          }
        }
      },
      {
        name: 'create_customer_record',
        taskReferenceName: 'create_customer',
        type: 'HTTP',
        description: 'Create customer record in CRM system',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-crm:4003/v1/customers',
            method: 'POST',
            body: {
              personalInfo: '${workflow.input.customerData.personalInfo}',
              kycData: '${kyc_check.output.response.body.verificationData}',
              status: 'active'
            }
          }
        }
      },
      {
        name: 'create_account',
        taskReferenceName: 'account_creation',
        type: 'HTTP',
        description: 'Create customer account in core system',
        inputParameters: {
          http_request: {
            uri: 'http://midaz-onboarding:3000/v1/accounts',
            method: 'POST',
            body: {
              customerId: '${create_customer.output.response.body.id}',
              accountType: '${workflow.input.accountType}',
              currency: '${workflow.input.defaultCurrency}'
            }
          }
        }
      },
      {
        name: 'send_welcome_notification',
        taskReferenceName: 'welcome_notification',
        type: 'HTTP',
        description: 'Send welcome notification to new customer',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-crm:4003/v1/notifications/send',
            method: 'POST',
            body: {
              recipient: '${workflow.input.customerData.email}',
              type: 'welcome',
              data: {
                customerName:
                  '${workflow.input.customerData.personalInfo.firstName}',
                accountId: '${account_creation.output.response.body.id}'
              }
            }
          }
        },
        optional: true
      }
    ],
    inputParameters: ['customerData', 'accountType', 'defaultCurrency'],
    outputParameters: ['customerId', 'accountId', 'status'],
    createdBy: 'onboarding@company.com',
    executionCount: 89,
    lastExecuted: '2025-01-01T12:15:00Z',
    avgExecutionTime: '45.2s',
    successRate: 0.921,
    createdAt: '2024-12-01T00:00:00Z',
    updatedAt: '2024-12-15T00:00:00Z',
    metadata: {
      category: 'onboarding',
      tags: ['kyc', 'onboarding', 'customer', 'account'],
      author: 'Onboarding Team',
      schemaVersion: '1.0',
      timeoutPolicy: {
        timeoutSeconds: 600,
        alertAfterTimeoutSeconds: 480
      },
      restartable: true,
      ownerEmail: 'onboarding@company.com'
    }
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231e',
    name: 'reconciliation_daily_batch',
    description: 'Daily batch reconciliation process for all transactions',
    version: 3,
    status: 'ACTIVE',
    tasks: [
      {
        name: 'fetch_external_transactions',
        taskReferenceName: 'external_data_fetch',
        type: 'HTTP',
        description: 'Fetch transactions from external banking systems',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-reconciliation:4004/v1/import/transactions',
            method: 'POST',
            body: {
              dateRange: {
                from: '${workflow.input.reconciliationDate}',
                to: '${workflow.input.reconciliationDate}'
              },
              sources: ['bank_api', 'payment_processor', 'card_network']
            }
          }
        },
        retryCount: 3,
        timeoutSeconds: 300
      },
      {
        name: 'fetch_internal_transactions',
        taskReferenceName: 'internal_data_fetch',
        type: 'HTTP',
        description: 'Fetch internal transaction records',
        inputParameters: {
          http_request: {
            uri: 'http://midaz-transaction:3001/v1/transactions/search',
            method: 'POST',
            body: {
              dateRange: {
                from: '${workflow.input.reconciliationDate}T00:00:00Z',
                to: '${workflow.input.reconciliationDate}T23:59:59Z'
              },
              status: 'completed'
            }
          }
        }
      },
      {
        name: 'run_reconciliation_matching',
        taskReferenceName: 'matching_process',
        type: 'HTTP',
        description: 'Run automatic matching algorithm',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-reconciliation:4004/v1/match',
            method: 'POST',
            body: {
              externalTransactions:
                '${external_data_fetch.output.response.body.transactions}',
              internalTransactions:
                '${internal_data_fetch.output.response.body.transactions}',
              matchingRules: '${workflow.input.matchingRules}'
            }
          }
        },
        retryCount: 1,
        timeoutSeconds: 600
      },
      {
        name: 'process_exceptions',
        taskReferenceName: 'exception_handling',
        type: 'HTTP',
        description: 'Process unmatched transactions and exceptions',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-reconciliation:4004/v1/exceptions/process',
            method: 'POST',
            body: {
              unmatchedTransactions:
                '${matching_process.output.response.body.unmatched}',
              autoResolveThreshold: 10.0
            }
          }
        }
      },
      {
        name: 'generate_reconciliation_report',
        taskReferenceName: 'report_generation',
        type: 'HTTP',
        description: 'Generate daily reconciliation report',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-smart-templates:4005/v1/reports/generate',
            method: 'POST',
            body: {
              templateId: 'daily_reconciliation_template',
              data: {
                date: '${workflow.input.reconciliationDate}',
                matches: '${matching_process.output.response.body.matched}',
                exceptions:
                  '${exception_handling.output.response.body.exceptions}'
              }
            }
          }
        }
      }
    ],
    inputParameters: ['reconciliationDate', 'matchingRules'],
    outputParameters: [
      'reportId',
      'matchedCount',
      'exceptionsCount',
      'reconciliationStatus'
    ],
    createdBy: 'reconciliation@company.com',
    executionCount: 45,
    lastExecuted: '2025-01-01T02:00:00Z',
    avgExecutionTime: '12m 34s',
    successRate: 0.956,
    createdAt: '2024-10-01T00:00:00Z',
    updatedAt: '2024-12-28T00:00:00Z',
    metadata: {
      category: 'reconciliation',
      tags: ['reconciliation', 'batch', 'reporting', 'matching'],
      author: 'Reconciliation Team',
      schemaVersion: '1.0',
      timeoutPolicy: {
        timeoutSeconds: 1800,
        alertAfterTimeoutSeconds: 1200
      },
      restartable: true,
      ownerEmail: 'reconciliation@company.com'
    }
  },
  {
    id: '01956b69-9102-75b7-8860-3e75c11d231f',
    name: 'compliance_audit_check',
    description: 'Automated compliance audit and reporting workflow',
    version: 1,
    status: 'DRAFT',
    tasks: [
      {
        name: 'collect_audit_data',
        taskReferenceName: 'data_collection',
        type: 'FORK_JOIN',
        description: 'Collect data from multiple sources in parallel',
        inputParameters: {
          dynamicForkJoinTasksParam: 'auditSources',
          dynamicForkJoinTasks: [
            {
              taskName: 'fetch_transaction_data',
              type: 'HTTP'
            },
            {
              taskName: 'fetch_customer_data',
              type: 'HTTP'
            },
            {
              taskName: 'fetch_compliance_records',
              type: 'HTTP'
            }
          ]
        }
      },
      {
        name: 'analyze_compliance_rules',
        taskReferenceName: 'compliance_analysis',
        type: 'HTTP',
        description: 'Analyze collected data against compliance rules',
        inputParameters: {
          http_request: {
            uri: 'http://plugin-compliance:4006/v1/analyze',
            method: 'POST',
            body: {
              auditData: '${data_collection.output}',
              rulesEngine: 'latest',
              auditPeriod: '${workflow.input.auditPeriod}'
            }
          }
        }
      }
    ],
    inputParameters: ['auditPeriod', 'complianceRules', 'auditSources'],
    outputParameters: ['auditReportId', 'complianceStatus', 'violations'],
    createdBy: 'compliance@company.com',
    executionCount: 0,
    successRate: 0,
    createdAt: '2024-12-20T00:00:00Z',
    updatedAt: '2024-12-20T00:00:00Z',
    metadata: {
      category: 'compliance',
      tags: ['audit', 'compliance', 'reporting', 'analysis'],
      author: 'Compliance Team',
      schemaVersion: '1.0',
      timeoutPolicy: {
        timeoutSeconds: 3600
      },
      restartable: false,
      ownerEmail: 'compliance@company.com'
    }
  }
]

// Mock Workflow Executions Data
export const mockWorkflowExecutions: WorkflowExecution[] = [
  {
    workflowId: '01956b69-9102-75b7-8860-3e75c11d2320',
    workflowName: 'payment_processing_flow',
    workflowVersion: 2,
    executionId: 'exec_01956b69-9102-75b7-8860-3e75c11d2320',
    status: 'COMPLETED',
    startTime: 1735740000000,
    endTime: 1735740023000,
    totalExecutionTime: 23000,
    input: {
      sourceAccount: 'acc_123456',
      destinationAccount: 'acc_789012',
      amount: 1500.0,
      currency: 'USD',
      customerEmail: 'customer@example.com',
      token: 'bearer_token_123'
    },
    output: {
      transactionId: 'txn_01956b69-9102-75b7-8860-3e75c11d2321',
      status: 'completed',
      fees: {
        amount: 15.0,
        currency: 'USD'
      }
    },
    tasks: [
      {
        taskId: 'task_001',
        taskType: 'HTTP',
        taskDefName: 'validate_accounts',
        referenceTaskName: 'account_validation',
        status: 'COMPLETED',
        startTime: 1735740001000,
        endTime: 1735740003000,
        executionTime: 2000,
        retryCount: 0,
        seq: 1,
        outputData: {
          response: {
            status: 'valid',
            accountDetails: {
              id: 'acc_123456',
              status: 'active',
              balance: 5000.0
            }
          }
        }
      },
      {
        taskId: 'task_002',
        taskType: 'HTTP',
        taskDefName: 'calculate_fees',
        referenceTaskName: 'fee_calculation',
        status: 'COMPLETED',
        startTime: 1735740003000,
        endTime: 1735740008000,
        executionTime: 5000,
        retryCount: 0,
        seq: 2,
        outputData: {
          response: {
            body: {
              status: 'approved',
              fees: {
                amount: 15.0,
                type: 'percentage',
                rate: 0.01
              }
            }
          }
        }
      },
      {
        taskId: 'task_003',
        taskType: 'SWITCH',
        taskDefName: 'fee_decision',
        referenceTaskName: 'fee_switch',
        status: 'COMPLETED',
        startTime: 1735740008000,
        endTime: 1735740009000,
        executionTime: 1000,
        retryCount: 0,
        seq: 3,
        outputData: {
          selectedCase: 'approved'
        }
      },
      {
        taskId: 'task_004',
        taskType: 'HTTP',
        taskDefName: 'create_transaction',
        referenceTaskName: 'transaction_creation',
        status: 'COMPLETED',
        startTime: 1735740009000,
        endTime: 1735740023000,
        executionTime: 14000,
        retryCount: 0,
        seq: 4,
        outputData: {
          response: {
            body: {
              id: 'txn_01956b69-9102-75b7-8860-3e75c11d2321',
              status: 'completed',
              createdAt: '2025-01-01T14:30:23Z'
            }
          }
        }
      }
    ],
    createdBy: 'api_user@company.com',
    priority: 0,
    correlationId: 'payment_batch_001'
  },
  {
    workflowId: '01956b69-9102-75b7-8860-3e75c11d2322',
    workflowName: 'payment_processing_flow',
    workflowVersion: 2,
    executionId: 'exec_01956b69-9102-75b7-8860-3e75c11d2322',
    status: 'FAILED',
    startTime: 1735739800000,
    endTime: 1735739815000,
    totalExecutionTime: 15000,
    input: {
      sourceAccount: 'acc_invalid',
      destinationAccount: 'acc_789012',
      amount: 2000.0,
      currency: 'USD',
      customerEmail: 'customer@example.com',
      token: 'bearer_token_123'
    },
    reasonForIncompletion: 'Account validation failed: Invalid account number',
    failedReferenceTaskNames: ['account_validation'],
    tasks: [
      {
        taskId: 'task_005',
        taskType: 'HTTP',
        taskDefName: 'validate_accounts',
        referenceTaskName: 'account_validation',
        status: 'FAILED',
        startTime: 1735739801000,
        endTime: 1735739815000,
        executionTime: 14000,
        retryCount: 3,
        seq: 1,
        reasonForIncompletion: 'HTTP 404: Account not found',
        outputData: {
          response: {
            error: 'Account not found',
            code: 'ACCOUNT_NOT_FOUND'
          }
        }
      }
    ],
    createdBy: 'api_user@company.com',
    priority: 0,
    correlationId: 'payment_batch_002'
  },
  {
    workflowId: '01956b69-9102-75b7-8860-3e75c11d2323',
    workflowName: 'customer_onboarding_flow',
    workflowVersion: 1,
    executionId: 'exec_01956b69-9102-75b7-8860-3e75c11d2323',
    status: 'RUNNING',
    startTime: 1735741200000,
    input: {
      customerData: {
        personalInfo: {
          firstName: 'John',
          lastName: 'Doe',
          email: 'john.doe@example.com',
          dateOfBirth: '1990-05-15'
        },
        documents: {
          nationalId: '123456789',
          passport: 'AB123456789'
        }
      },
      accountType: 'checking',
      defaultCurrency: 'USD'
    },
    tasks: [
      {
        taskId: 'task_006',
        taskType: 'HTTP',
        taskDefName: 'kyc_verification',
        referenceTaskName: 'kyc_check',
        status: 'COMPLETED',
        startTime: 1735741201000,
        endTime: 1735741245000,
        executionTime: 44000,
        retryCount: 0,
        seq: 1,
        outputData: {
          response: {
            body: {
              status: 'approved',
              verificationData: {
                score: 95,
                level: 'full_verification'
              }
            }
          }
        }
      },
      {
        taskId: 'task_007',
        taskType: 'SWITCH',
        taskDefName: 'kyc_decision',
        referenceTaskName: 'kyc_switch',
        status: 'COMPLETED',
        startTime: 1735741245000,
        endTime: 1735741246000,
        executionTime: 1000,
        retryCount: 0,
        seq: 2,
        outputData: {
          selectedCase: 'approved'
        }
      },
      {
        taskId: 'task_008',
        taskType: 'HTTP',
        taskDefName: 'create_customer_record',
        referenceTaskName: 'create_customer',
        status: 'IN_PROGRESS',
        startTime: 1735741246000,
        retryCount: 0,
        seq: 3
      }
    ],
    createdBy: 'onboarding_api@company.com',
    priority: 1,
    correlationId: 'onboarding_batch_003'
  }
]

// Mock Workflow Templates Data
export const mockWorkflowTemplates: WorkflowTemplate[] = [
  {
    id: 'tmpl_payment_processing',
    name: 'Payment Processing Template',
    description:
      'Standard payment processing workflow with validation, fees, and transaction creation',
    category: 'payments',
    tags: ['payment', 'transaction', 'fees', 'validation'],
    workflow: {
      name: 'payment_processing_template',
      description: 'Template for payment processing workflows',
      tasks: [
        {
          name: 'validate_accounts',
          type: 'HTTP',
          description: 'Validate source and destination accounts',
          configurable: {
            endpoint: true,
            method: false,
            headers: true,
            timeout: true
          }
        },
        {
          name: 'calculate_fees',
          type: 'HTTP',
          description: 'Calculate applicable fees for the transaction',
          configurable: {
            endpoint: true,
            body: true,
            timeout: true
          }
        },
        {
          name: 'create_transaction',
          type: 'HTTP',
          description: 'Create the actual transaction',
          configurable: {
            endpoint: true,
            body: true,
            retries: true
          }
        },
        {
          name: 'send_confirmation',
          type: 'HTTP',
          description: 'Send confirmation notification',
          optional: true,
          configurable: {
            endpoint: true,
            body: true
          }
        }
      ],
      inputParameters: [
        'sourceAccount',
        'destinationAccount',
        'amount',
        'currency'
      ],
      timeoutSeconds: 300
    },
    parameters: [
      {
        name: 'sourceAccount',
        type: 'string',
        required: true,
        description: 'Source account identifier',
        validation: {
          pattern: '^acc_[a-zA-Z0-9]{6,}$'
        }
      },
      {
        name: 'destinationAccount',
        type: 'string',
        required: true,
        description: 'Destination account identifier',
        validation: {
          pattern: '^acc_[a-zA-Z0-9]{6,}$'
        }
      },
      {
        name: 'amount',
        type: 'number',
        required: true,
        description: 'Transaction amount',
        validation: {
          min: 0.01,
          max: 1000000
        }
      },
      {
        name: 'currency',
        type: 'select',
        required: true,
        description: 'Currency code',
        options: [
          { label: 'US Dollar', value: 'USD' },
          { label: 'Euro', value: 'EUR' },
          { label: 'British Pound', value: 'GBP' },
          { label: 'Japanese Yen', value: 'JPY' }
        ]
      }
    ],
    usageCount: 156,
    rating: 4.8,
    createdBy: 'template_admin@company.com',
    createdAt: '2024-10-15T00:00:00Z',
    updatedAt: '2024-12-10T00:00:00Z',
    isPublic: true,
    metadata: {
      version: '2.0',
      schemaVersion: '1.0',
      complexity: 'MEDIUM',
      estimatedDuration: '30-60 seconds',
      requiredServices: [
        'midaz-onboarding',
        'plugin-fees',
        'midaz-transaction'
      ],
      supportedFormats: ['json'],
      documentation:
        'Complete payment processing workflow with built-in validation and fee calculation.',
      examples: [
        {
          name: 'Simple Transfer',
          description: 'Basic account-to-account transfer',
          input: {
            sourceAccount: 'acc_123456',
            destinationAccount: 'acc_789012',
            amount: 100.0,
            currency: 'USD'
          },
          expectedOutput: {
            transactionId: 'txn_abc123',
            status: 'completed',
            fees: { amount: 1.0, currency: 'USD' }
          }
        }
      ]
    }
  },
  {
    id: 'tmpl_customer_onboarding',
    name: 'Customer Onboarding Template',
    description:
      'Complete customer onboarding workflow with KYC, account creation, and notifications',
    category: 'onboarding',
    tags: ['kyc', 'onboarding', 'customer', 'account'],
    workflow: {
      name: 'customer_onboarding_template',
      description: 'Template for customer onboarding workflows',
      tasks: [
        {
          name: 'kyc_verification',
          type: 'HTTP',
          description: 'Perform KYC verification',
          configurable: {
            endpoint: true,
            body: true,
            timeout: true
          }
        },
        {
          name: 'kyc_decision',
          type: 'SWITCH',
          description: 'Decision based on KYC results',
          configurable: {
            body: true
          }
        },
        {
          name: 'create_customer_record',
          type: 'HTTP',
          description: 'Create customer record in CRM',
          configurable: {
            endpoint: true,
            body: true
          }
        },
        {
          name: 'create_account',
          type: 'HTTP',
          description: 'Create customer account',
          configurable: {
            endpoint: true,
            body: true
          }
        }
      ],
      inputParameters: ['customerData', 'accountType'],
      timeoutSeconds: 600
    },
    parameters: [
      {
        name: 'customerData',
        type: 'object',
        required: true,
        description: 'Customer information for onboarding'
      },
      {
        name: 'accountType',
        type: 'select',
        required: true,
        description: 'Type of account to create',
        options: [
          { label: 'Checking Account', value: 'checking' },
          { label: 'Savings Account', value: 'savings' },
          { label: 'Business Account', value: 'business' }
        ]
      }
    ],
    usageCount: 43,
    rating: 4.6,
    createdBy: 'onboarding_admin@company.com',
    createdAt: '2024-11-20T00:00:00Z',
    updatedAt: '2024-12-18T00:00:00Z',
    isPublic: true,
    metadata: {
      version: '1.0',
      schemaVersion: '1.0',
      complexity: 'COMPLEX',
      estimatedDuration: '2-5 minutes',
      requiredServices: ['plugin-identity', 'plugin-crm', 'midaz-onboarding'],
      supportedFormats: ['json'],
      documentation:
        'Comprehensive customer onboarding with KYC verification and account setup.'
    }
  },
  {
    id: 'tmpl_reconciliation_batch',
    name: 'Daily Reconciliation Template',
    description:
      'Automated daily reconciliation process with exception handling',
    category: 'reconciliation',
    tags: ['reconciliation', 'batch', 'reporting', 'automation'],
    workflow: {
      name: 'reconciliation_batch_template',
      description: 'Template for daily reconciliation processes',
      tasks: [
        {
          name: 'fetch_external_data',
          type: 'HTTP',
          description: 'Fetch external transaction data',
          configurable: {
            endpoint: true,
            body: true,
            timeout: true
          }
        },
        {
          name: 'fetch_internal_data',
          type: 'HTTP',
          description: 'Fetch internal transaction data',
          configurable: {
            endpoint: true,
            body: true
          }
        },
        {
          name: 'run_matching',
          type: 'HTTP',
          description: 'Run reconciliation matching',
          configurable: {
            body: true,
            timeout: true
          }
        },
        {
          name: 'generate_report',
          type: 'HTTP',
          description: 'Generate reconciliation report',
          configurable: {
            endpoint: true,
            body: true
          }
        }
      ],
      inputParameters: ['reconciliationDate', 'matchingRules'],
      timeoutSeconds: 1800
    },
    parameters: [
      {
        name: 'reconciliationDate',
        type: 'string',
        required: true,
        description: 'Date for reconciliation (YYYY-MM-DD format)',
        validation: {
          pattern: '^\d{4}-\d{2}-\d{2}$'
        }
      },
      {
        name: 'matchingRules',
        type: 'object',
        required: false,
        description: 'Custom matching rules for reconciliation'
      }
    ],
    usageCount: 28,
    rating: 4.4,
    createdBy: 'reconciliation_admin@company.com',
    createdAt: '2024-09-10T00:00:00Z',
    updatedAt: '2024-12-05T00:00:00Z',
    isPublic: true,
    metadata: {
      version: '3.0',
      schemaVersion: '1.0',
      complexity: 'ADVANCED',
      estimatedDuration: '10-30 minutes',
      requiredServices: [
        'plugin-reconciliation',
        'midaz-transaction',
        'plugin-smart-templates'
      ],
      supportedFormats: ['json'],
      documentation:
        'Advanced reconciliation workflow with parallel processing and intelligent matching.'
    }
  }
]
