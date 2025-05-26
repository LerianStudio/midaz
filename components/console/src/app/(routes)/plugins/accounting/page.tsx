import React from 'react'
import { Metadata } from 'next'
import Link from 'next/link'

export const metadata: Metadata = {
  title: 'Accounting - Midaz Console',
  description:
    'Chart of accounts management, transaction routes, and financial compliance'
}

export default function AccountingPage() {
  return (
    <div className="p-6">
      <div className="space-y-6">
        {/* Overview Header */}
        <div className="space-y-2">
          <h1 className="text-3xl font-bold tracking-tight">
            Accounting Overview
          </h1>
          <p className="text-muted-foreground">
            Manage chart of accounts, transaction routes, and ensure financial
            compliance
          </p>
        </div>

        {/* Quick Stats */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <div className="rounded-lg border bg-card p-6 text-card-foreground shadow-sm">
            <div className="flex flex-row items-center justify-between space-y-0 pb-2">
              <h3 className="text-sm font-medium">Account Types</h3>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                className="h-4 w-4 text-muted-foreground"
              >
                <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
            </div>
            <div className="space-y-1">
              <div className="text-2xl font-bold">15</div>
              <p className="text-xs text-muted-foreground">
                12 active, 3 inactive
              </p>
            </div>
          </div>

          <div className="rounded-lg border bg-card p-6 text-card-foreground shadow-sm">
            <div className="flex flex-row items-center justify-between space-y-0 pb-2">
              <h3 className="text-sm font-medium">Transaction Routes</h3>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                className="h-4 w-4 text-muted-foreground"
              >
                <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
                <polyline points="3.29,7 12,12 20.71,7" />
                <line x1="12" x2="12" y1="22" y2="12" />
              </svg>
            </div>
            <div className="space-y-1">
              <div className="text-2xl font-bold">8</div>
              <p className="text-xs text-muted-foreground">6 active, 2 draft</p>
            </div>
          </div>

          <div className="rounded-lg border bg-card p-6 text-card-foreground shadow-sm">
            <div className="flex flex-row items-center justify-between space-y-0 pb-2">
              <h3 className="text-sm font-medium">Operation Routes</h3>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                className="h-4 w-4 text-muted-foreground"
              >
                <polyline points="16,18 22,12 16,6" />
                <polyline points="8,6 2,12 8,18" />
              </svg>
            </div>
            <div className="space-y-1">
              <div className="text-2xl font-bold">24</div>
              <p className="text-xs text-muted-foreground">All operational</p>
            </div>
          </div>

          <div className="rounded-lg border bg-card p-6 text-card-foreground shadow-sm">
            <div className="flex flex-row items-center justify-between space-y-0 pb-2">
              <h3 className="text-sm font-medium">Compliance Score</h3>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                className="h-4 w-4 text-muted-foreground"
              >
                <path d="M9 12l2 2 4-4" />
                <path d="M21 12c-1 0-3-1-3-3s2-3 3-3 3 1 3 3-2 3-3 3" />
                <path d="M3 12c1 0 3-1 3-3s-2-3-3-3-3 1-3 3 2 3 3 3" />
                <path d="M3 12c0 5.5 4.5 10 10 10s10-4.5 10-10" />
              </svg>
            </div>
            <div className="space-y-1">
              <div className="text-2xl font-bold text-green-600">96.5%</div>
              <p className="text-xs text-muted-foreground">Excellent</p>
            </div>
          </div>
        </div>

        {/* Quick Actions */}
        <div className="rounded-lg border bg-card p-6">
          <h2 className="mb-4 text-lg font-semibold">Quick Actions</h2>
          <div className="grid gap-4 md:grid-cols-3">
            <Link
              href="/plugins/accounting/account-types/create"
              className="flex items-center space-x-3 rounded-lg border p-4 transition-colors hover:bg-muted"
            >
              <div className="rounded-full bg-blue-100 p-2">
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2"
                  className="h-4 w-4 text-blue-600"
                >
                  <circle cx="12" cy="12" r="10" />
                  <path d="M8 12h8" />
                  <path d="M12 8v8" />
                </svg>
              </div>
              <div>
                <h3 className="font-medium">Create Account Type</h3>
                <p className="text-sm text-muted-foreground">
                  Add new account type to chart
                </p>
              </div>
            </Link>

            <Link
              href="/plugins/accounting/transaction-routes/create"
              className="flex items-center space-x-3 rounded-lg border p-4 transition-colors hover:bg-muted"
            >
              <div className="rounded-full bg-green-100 p-2">
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2"
                  className="h-4 w-4 text-green-600"
                >
                  <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
                  <polyline points="3.29,7 12,12 20.71,7" />
                  <line x1="12" x2="12" y1="22" y2="12" />
                </svg>
              </div>
              <div>
                <h3 className="font-medium">Design Transaction Route</h3>
                <p className="text-sm text-muted-foreground">
                  Create accounting template
                </p>
              </div>
            </Link>

            <Link
              href="/plugins/accounting/compliance"
              className="flex items-center space-x-3 rounded-lg border p-4 transition-colors hover:bg-muted"
            >
              <div className="rounded-full bg-purple-100 p-2">
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2"
                  className="h-4 w-4 text-purple-600"
                >
                  <path d="M9 12l2 2 4-4" />
                  <path d="M21 12c-1 0-3-1-3-3s2-3 3-3 3 1 3 3-2 3-3 3" />
                  <path d="M3 12c1 0 3-1 3-3s-2-3-3-3-3 1-3 3 2 3 3 3" />
                  <path d="M3 12c0 5.5 4.5 10 10 10s10-4.5 10-10" />
                </svg>
              </div>
              <div>
                <h3 className="font-medium">Review Compliance</h3>
                <p className="text-sm text-muted-foreground">
                  Check validation status
                </p>
              </div>
            </Link>
          </div>
        </div>

        {/* Recent Activity */}
        <div className="rounded-lg border bg-card p-6">
          <h2 className="mb-4 text-lg font-semibold">Recent Activity</h2>
          <div className="space-y-3">
            <div className="flex items-center space-x-3 text-sm">
              <div className="h-2 w-2 rounded-full bg-green-500"></div>
              <span className="text-muted-foreground">2 hours ago</span>
              <span>Created account type "Business Checking" (BCHCK)</span>
            </div>
            <div className="flex items-center space-x-3 text-sm">
              <div className="h-2 w-2 rounded-full bg-blue-500"></div>
              <span className="text-muted-foreground">4 hours ago</span>
              <span>
                Updated transaction route "Wire Transfer" validation rules
              </span>
            </div>
            <div className="flex items-center space-x-3 text-sm">
              <div className="h-2 w-2 rounded-full bg-yellow-500"></div>
              <span className="text-muted-foreground">1 day ago</span>
              <span>Compliance validation completed - 96.5% score</span>
            </div>
            <div className="flex items-center space-x-3 text-sm">
              <div className="h-2 w-2 rounded-full bg-purple-500"></div>
              <span className="text-muted-foreground">2 days ago</span>
              <span>Created operation route for external bank transfers</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
