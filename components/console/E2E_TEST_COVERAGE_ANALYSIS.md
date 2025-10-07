# Midaz Console E2E Test Coverage Analysis

**Generated:** October 6, 2025
**Repository:** /home/paulobarbosa/Documentos/www/midaz/components/console

---

## 1. E2E Test Coverage Summary

### Overall Statistics

- **Total Routes with page.tsx:** 11
- **Routes with Tests:** 10
- **Routes without Tests:** 1
- **Coverage Percentage:** 90.9%

### Test File Count

- **Total E2E Test Files:** 13
- **Route-specific Tests:** 10
- **Utility/Infrastructure Tests:** 3

---

## 2. Detailed Route Coverage

### Routes WITH Tests ✅

| Route                 | Test File                                                   | Coverage Status              |
| --------------------- | ----------------------------------------------------------- | ---------------------------- |
| `/accounts`           | `accounts-flow.spec.ts`                                     | ✅ Full CRUD                 |
| `/account-types`      | `account-types-flow.spec.ts`                                | ✅ Full CRUD                 |
| `/assets`             | `assets-flow.spec.ts`                                       | ✅ Full CRUD                 |
| `/ledgers`            | `ledger-flow.spec.ts`, `ledgers-comprehensive-flow.spec.ts` | ✅ Full CRUD + Comprehensive |
| `/onboarding`         | `onboarding-flow.spec.ts`                                   | ✅ Full flow                 |
| `/operation-routes`   | `operation-routes-flow.spec.ts`                             | ✅ Full CRUD                 |
| `/portfolios`         | `portfolios-flow.spec.ts`                                   | ✅ Full CRUD                 |
| `/segments`           | `segments-flow.spec.ts`                                     | ✅ Full CRUD                 |
| `/transaction-routes` | `transaction-routes-flow.spec.ts`                           | ✅ Full CRUD                 |
| `/transactions`       | `transactions-flow.spec.ts`                                 | ✅ Full CRUD + Details       |

### Routes WITHOUT Tests ❌

| Route       | Sub-routes                                             | Priority | Reason                                |
| ----------- | ------------------------------------------------------ | -------- | ------------------------------------- |
| `/settings` | `/organizations`, `/users`, `/applications`, `/system` | **HIGH** | Complex multi-tab interface with RBAC |

### Utility Tests

| Test File                    | Purpose                   |
| ---------------------------- | ------------------------- |
| `login-redirection.spec.ts`  | Authentication flow       |
| `sidebar-navigation.spec.ts` | Navigation infrastructure |

---

## 3. Missing E2E Tests - Priority Analysis

### 3.1 HIGH Priority: Settings Route

**File Path:** `/home/paulobarbosa/Documentos/www/midaz/components/console/src/app/(routes)/settings/page.tsx`

**Complexity:** High - Tab-based interface with multiple sub-sections

**Sub-routes:**

1. `/settings/organizations` - Organization CRUD
2. `/settings/organizations/[id]` - Organization details/edit
3. `/settings/organizations/new-organization` - Organization creation
4. `/settings?tab=users` - Users management (RBAC protected)
5. `/settings?tab=applications` - Applications management (RBAC protected)
6. `/settings?tab=system` - System configuration

**Why High Priority:**

- Critical business functionality (organization management)
- RBAC enforcement testing required
- Multiple user flows and permissions
- Data integrity critical for multi-tenant system

---

## 4. Client APIs Without Route Tests

### APIs with Client Implementation but No Dedicated Route

| Client API                        | Route Exists?      | Test Needed? | Notes                                      |
| --------------------------------- | ------------------ | ------------ | ------------------------------------------ |
| `balances.ts`                     | No                 | **YES**      | Used by accounts - integration test needed |
| `groups.ts`                       | No                 | Unknown      | Verify if feature is implemented           |
| `home.ts`                         | Yes (`/page.tsx`)  | **YES**      | Home page metrics display                  |
| `transaction-operation-routes.ts` | No                 | **YES**      | Related to transactions - verify usage     |
| `applications.ts`                 | Yes (Settings tab) | **YES**      | Part of settings                           |
| `users.ts`                        | Yes (Settings tab) | **YES**      | Part of settings                           |
| `organizations.ts`                | Yes (Settings tab) | **YES**      | Part of settings                           |
| `midaz-config.ts`                 | No                 | Maybe        | System configuration                       |
| `midaz-info.ts`                   | No                 | Maybe        | System information                         |
| `plugin-menu.ts`                  | No                 | Maybe        | UI infrastructure                          |

---

## 5. Recommended Test Structure

### 5.1 Settings Route Test (`settings-flow.spec.ts`)

```typescript
import { test, expect } from '@playwright/test'
import { navigateToSettings } from '../utils/navigate-to-settings'

test.beforeEach(async ({ page }) => {
  await navigateToSettings(page)
})

test.describe('Settings Management - E2E Tests', () => {
  test.describe('Organizations Tab', () => {
    test('should display organizations list', async ({ page }) => {
      // Verify organizations tab is active by default
      // Verify organizations table/list is visible
    })

    test('should create new organization', async ({ page }) => {
      // Click "New Organization" button
      // Fill organization form
      // Submit and verify creation
    })

    test('should edit organization details', async ({ page }) => {
      // Select existing organization
      // Navigate to edit page
      // Update fields
      // Save and verify changes
    })

    test('should delete organization', async ({ page }) => {
      // Create test organization
      // Delete organization
      // Confirm deletion
      // Verify removal from list
    })

    test('should handle organization metadata', async ({ page }) => {
      // Add metadata to organization
      // Verify metadata is saved
    })
  })

  test.describe('Users Tab', () => {
    test('should enforce RBAC permissions', async ({ page }) => {
      // Verify tab is hidden without proper permissions
      // Verify tab is visible with permissions
    })

    test('should list users', async ({ page }) => {
      // Navigate to users tab
      // Verify users list displays
    })

    test('should create new user', async ({ page }) => {
      // Open create user form
      // Fill required fields (username, email, password, roles)
      // Submit and verify creation
    })

    test('should update user details', async ({ page }) => {
      // Select user
      // Update user information
      // Save and verify changes
    })

    test('should update user password', async ({ page }) => {
      // Select user
      // Change password
      // Verify password change
    })

    test('should reset user password (admin)', async ({ page }) => {
      // Admin password reset functionality
      // Verify reset token/email
    })

    test('should delete user', async ({ page }) => {
      // Create test user
      // Delete user
      // Confirm deletion
    })

    test('should validate user form fields', async ({ page }) => {
      // Test required field validation
      // Test email format validation
      // Test password strength validation
    })
  })

  test.describe('Applications Tab', () => {
    test('should enforce RBAC permissions', async ({ page }) => {
      // Verify tab is hidden without proper permissions
    })

    test('should list applications', async ({ page }) => {
      // Navigate to applications tab
      // Verify applications list displays
    })

    test('should create new application', async ({ page }) => {
      // Open create application form
      // Fill application details
      // Submit and verify creation
    })

    test('should display application credentials', async ({ page }) => {
      // Create application
      // Verify client ID and secret are displayed
      // Verify security warning is shown
    })

    test('should delete application', async ({ page }) => {
      // Create test application
      // Delete application
      // Confirm deletion with warning
    })

    test('should handle application security alerts', async ({ page }) => {
      // Verify security alert for credentials
      // Test copy to clipboard functionality
    })
  })

  test.describe('System Tab', () => {
    test('should display system information', async ({ page }) => {
      // Navigate to system tab
      // Verify system configuration displays
    })

    test('should update system settings', async ({ page }) => {
      // Update system configuration
      // Save and verify changes
    })
  })

  test.describe('Tab Navigation', () => {
    test('should switch between tabs using URL params', async ({ page }) => {
      // Navigate to /settings?tab=users
      // Verify users tab is active
      // Navigate to /settings?tab=applications
      // Verify applications tab is active
    })

    test('should maintain state when switching tabs', async ({ page }) => {
      // Fill form in one tab
      // Switch to another tab
      // Return to original tab
      // Verify form state is maintained
    })

    test('should handle invalid tab parameter', async ({ page }) => {
      // Navigate to /settings?tab=invalid
      // Verify default tab (organizations) is shown
    })
  })

  test.describe('Breadcrumb Navigation', () => {
    test('should display correct breadcrumbs', async ({ page }) => {
      // Verify breadcrumb shows "Settings"
      // Verify active tab name in breadcrumb
    })

    test('should update breadcrumb on tab change', async ({ page }) => {
      // Switch tabs
      // Verify breadcrumb updates
    })
  })
})
```

---

### 5.2 Home Page Test (`home-flow.spec.ts`)

```typescript
import { test, expect } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.goto('/')
})

test.describe('Home Page - E2E Tests', () => {
  test.describe('Page Layout', () => {
    test('should display ledger name in header', async ({ page }) => {
      // Verify current ledger name is displayed
      // Or skeleton if loading
    })

    test('should display welcome message', async ({ page }) => {
      // Verify home page description
    })

    test('should display decorative graphics', async ({ page }) => {
      // Verify images are loaded
    })
  })

  test.describe('Next Steps Cards', () => {
    test('should display all three next step cards', async ({ page }) => {
      // Assets card
      // Accounts card
      // Transactions card
    })

    test('should navigate to assets page', async ({ page }) => {
      // Click "Manage Assets" button
      // Verify navigation to /assets
    })

    test('should navigate to accounts page', async ({ page }) => {
      // Click "Manage Accounts" button
      // Verify navigation to /accounts
    })

    test('should navigate to transactions page', async ({ page }) => {
      // Click "View Transactions" button
      // Verify navigation to /transactions
    })

    test('should disable cards when no organization/ledger', async ({
      page
    }) => {
      // Clear organization/ledger context
      // Verify buttons are disabled
      // Verify tooltip shows explanation
    })
  })

  test.describe('Metrics Section', () => {
    test('should display operation metrics', async ({ page }) => {
      // Verify metrics section is visible
      // Verify data is loaded from home.ts API
    })

    test('should handle loading state', async ({ page }) => {
      // Verify skeleton/loading state
    })

    test('should handle error state', async ({ page }) => {
      // Simulate API error
      // Verify error message
    })
  })

  test.describe('Footer Section', () => {
    test('should display dev resources links', async ({ page }) => {
      // Verify Lerian Docs link
      // Verify Lerian Discord link
      // Verify Github link
    })

    test('should open external links in new tab', async ({ page }) => {
      // Click external link
      // Verify target="_blank"
    })

    test('should display blog banner', async ({ page }) => {
      // Verify banner image
      // Verify "Check out our Blog" button
    })

    test('should navigate to blog', async ({ page }) => {
      // Click blog button
      // Verify navigation to dev.to/lerian
    })
  })
})
```

---

### 5.3 Balances Integration Test (`balances-integration.spec.ts`)

```typescript
import { test, expect } from '@playwright/test'
import { navigateToAccounts } from '../utils/navigate-to-accounts'

test.describe('Balances Integration - E2E Tests', () => {
  test('should display account balances', async ({ page }) => {
    await navigateToAccounts(page)

    // Click on an account
    const firstAccount = page.getByTestId('account-row').first()
    await firstAccount.click()

    // Verify balance information is displayed
    await expect(page.getByTestId('account-balance')).toBeVisible()
  })

  test('should show balance by asset', async ({ page }) => {
    // Navigate to account details
    // Verify balances are grouped by asset
    // Verify balance amounts are correct
  })

  test('should update balance after transaction', async ({ page }) => {
    // Note initial balance
    // Create transaction affecting this account
    // Verify balance is updated
  })

  test('should handle multiple asset balances', async ({ page }) => {
    // Account with multiple assets
    // Verify all balances are displayed
    // Verify correct formatting
  })
})
```

---

### 5.4 Authentication & Authorization Test (Enhancement to existing)

```typescript
// Enhance login-redirection.spec.ts

test.describe('Authorization & Permissions', () => {
  test('should enforce RBAC on settings tabs', async ({ page }) => {
    // Login as user without user management permissions
    // Navigate to settings
    // Verify users tab is hidden
    // Login as admin
    // Verify users tab is visible
  })

  test('should restrict actions based on permissions', async ({ page }) => {
    // Test create/edit/delete permissions per resource
  })
})
```

---

## 6. Edge Cases & Error Scenarios to Test

### 6.1 Settings Route Edge Cases

1. **Organization Management:**
   - Create organization with duplicate name
   - Create organization with invalid data
   - Delete organization with active ledgers
   - Update organization with concurrent edits

2. **User Management:**
   - Create user with existing email
   - Create user with weak password
   - Delete user with active sessions
   - Update user roles and verify permission changes

3. **Application Management:**
   - Regenerate application credentials
   - Delete application with active tokens
   - Copy credentials to clipboard

### 6.2 Data Integrity Tests

1. **Cross-Entity Dependencies:**
   - Delete organization → verify cascade to ledgers
   - Delete user → verify sessions are invalidated
   - Delete application → verify tokens are revoked

2. **Concurrent Operations:**
   - Multiple users editing same entity
   - Race conditions in create operations

---

## 7. Test Utilities Needed

### Navigation Utilities (to create)

```typescript
// tests/utils/navigate-to-settings.ts
export async function navigateToSettings(page: Page) {
  await page.goto('/settings')
  await page.waitForLoadState('networkidle')
}

export async function navigateToSettingsTab(
  page: Page,
  tab: 'organizations' | 'users' | 'applications' | 'system'
) {
  await page.goto(`/settings?tab=${tab}`)
  await page.waitForLoadState('networkidle')
}
```

### Helper Functions

```typescript
// tests/helpers/settings-helpers.ts
export async function createTestOrganization(page: Page, name: string) {
  // Implementation
}

export async function createTestUser(page: Page, userData: any) {
  // Implementation
}

export async function createTestApplication(page: Page, appData: any) {
  // Implementation
}
```

---

## 8. Testing Strategy Recommendations

### 8.1 Test Execution Order

1. **Infrastructure Tests** (Run first)
   - `login-redirection.spec.ts`
   - `sidebar-navigation.spec.ts`

2. **Setup Tests** (Required state)
   - `onboarding-flow.spec.ts`
   - `settings-flow.spec.ts` (Organizations)
   - `ledgers-comprehensive-flow.spec.ts`

3. **Entity Tests** (Core features)
   - `assets-flow.spec.ts`
   - `accounts-flow.spec.ts`
   - `account-types-flow.spec.ts`
   - `portfolios-flow.spec.ts`
   - `segments-flow.spec.ts`

4. **Advanced Tests** (Complex workflows)
   - `transactions-flow.spec.ts`
   - `transaction-routes-flow.spec.ts`
   - `operation-routes-flow.spec.ts`

### 8.2 Test Data Management

**Recommendation:** Implement test data factories

```typescript
// tests/factories/organization.factory.ts
export class OrganizationFactory {
  static create(overrides?: Partial<OrganizationData>) {
    return {
      legalName: `Test Org ${Date.now()}`,
      legalDocument: generateDocument(),
      ...overrides
    }
  }
}
```

### 8.3 Parallel Execution Considerations

- **Isolate test data** - Use unique identifiers per test
- **Avoid shared state** - Each test should be independent
- **Clean up resources** - Delete test data after test completion

---

## 9. Performance & Reliability Improvements

### 9.1 Common Patterns to Standardize

1. **Waiting Strategies:**

   ```typescript
   // Good
   await page.waitForLoadState('networkidle')
   await expect(element).toBeVisible({ timeout: 15000 })

   // Avoid
   await page.waitForTimeout(5000) // Only when absolutely necessary
   ```

2. **Element Locators:**
   - Prefer `data-testid` for stable selectors
   - Use role-based selectors for accessibility
   - Avoid CSS selectors dependent on structure

3. **Error Handling:**
   ```typescript
   const isVisible = await element.isVisible().catch(() => false)
   if (isVisible) {
     // Handle element presence
   }
   ```

### 9.2 Flakiness Reduction

1. **Add explicit waits** before interactions
2. **Verify element state** (visible, enabled) before clicking
3. **Use retry logic** for transient failures
4. **Mock time-dependent operations** where appropriate

---

## 10. CI/CD Integration Recommendations

### 10.1 Test Suites

```typescript
// playwright.config.ts
export default defineConfig({
  projects: [
    {
      name: 'smoke',
      testMatch: /.*\.(smoke|critical)\.spec\.ts/,
      retries: 2
    },
    {
      name: 'full',
      testMatch: /.*\.spec\.ts/,
      retries: 1
    },
    {
      name: 'settings',
      testMatch: /settings.*\.spec\.ts/,
      retries: 2
    }
  ]
})
```

### 10.2 Test Execution Strategy

- **PR Checks:** Run smoke tests (critical paths)
- **Nightly Builds:** Run full test suite
- **Release:** Run full suite + performance tests

---

## 11. Metrics to Track

### 11.1 Coverage Metrics

| Metric                  | Current | Target |
| ----------------------- | ------- | ------ |
| Route Coverage          | 90.9%   | 100%   |
| CRUD Coverage           | High    | High   |
| Error Scenario Coverage | Medium  | High   |
| RBAC Coverage           | Low     | High   |
| Integration Coverage    | Low     | Medium |

### 11.2 Quality Metrics

- Test execution time
- Flakiness rate (< 5% target)
- Test maintenance cost
- Bug escape rate

---

## 12. Next Steps

### Immediate Actions (Week 1-2)

1. ✅ **Create `settings-flow.spec.ts`** - HIGH PRIORITY
   - Organizations CRUD
   - Users management (RBAC)
   - Applications management (RBAC)
   - System tab

2. ✅ **Create `home-flow.spec.ts`** - MEDIUM PRIORITY
   - Page layout and navigation
   - Next steps cards functionality
   - Metrics display
   - Footer links

3. ✅ **Create navigation utilities** for settings
   - `navigate-to-settings.ts`
   - Tab-specific navigation helpers

### Short-term Actions (Week 3-4)

4. ✅ **Create `balances-integration.spec.ts`**
   - Balance display in account details
   - Balance updates after transactions

5. ✅ **Enhance authentication tests**
   - RBAC enforcement
   - Permission-based UI hiding

6. ✅ **Create test data factories**
   - Organization factory
   - User factory
   - Application factory

### Long-term Actions (Month 2+)

7. ✅ **Performance testing**
   - Large dataset handling
   - Pagination performance
   - API response times

8. ✅ **Accessibility testing**
   - Keyboard navigation
   - Screen reader compatibility
   - ARIA labels

9. ✅ **Mobile responsive testing**
   - Mobile viewport tests
   - Touch interactions

---

## 13. Conclusion

The Midaz Console has **excellent E2E test coverage** at 90.9%, with comprehensive tests for most routes. The primary gap is the **Settings route**, which is critical due to its:

- Multi-tenant organization management
- User management with RBAC
- Application credential management
- System configuration

**Priority Recommendation:** Implement settings route tests immediately to achieve 100% route coverage and ensure proper RBAC enforcement testing.

**Estimated Effort:**

- Settings tests: 2-3 days
- Home page tests: 1 day
- Integration tests: 1-2 days
- Test utilities & refactoring: 1 day

**Total:** ~5-7 days to achieve comprehensive coverage

---

## Appendix A: File Structure

```
tests/
├── e2e/
│   ├── accounts-flow.spec.ts ✅
│   ├── account-types-flow.spec.ts ✅
│   ├── assets-flow.spec.ts ✅
│   ├── balances-integration.spec.ts ❌ (TO CREATE)
│   ├── home-flow.spec.ts ❌ (TO CREATE)
│   ├── ledger-flow.spec.ts ✅
│   ├── ledgers-comprehensive-flow.spec.ts ✅
│   ├── login-redirection.spec.ts ✅
│   ├── onboarding-flow.spec.ts ✅
│   ├── operation-routes-flow.spec.ts ✅
│   ├── portfolios-flow.spec.ts ✅
│   ├── segments-flow.spec.ts ✅
│   ├── settings-flow.spec.ts ❌ (TO CREATE - HIGH PRIORITY)
│   ├── sidebar-navigation.spec.ts ✅
│   ├── transaction-routes-flow.spec.ts ✅
│   └── transactions-flow.spec.ts ✅
├── utils/
│   ├── navigate-to-accounts.ts ✅
│   ├── navigate-to-settings.ts ❌ (TO CREATE)
│   └── ... (other navigation utilities)
└── helpers/
    └── settings-helpers.ts ❌ (TO CREATE)
```

---

## Appendix B: Test Coverage Matrix

| Feature            | List | Create | Read | Update | Delete | Validate | RBAC |
| ------------------ | ---- | ------ | ---- | ------ | ------ | -------- | ---- |
| Accounts           | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Account Types      | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Assets             | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Ledgers            | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Portfolios         | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Segments           | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Transactions       | ✅   | ✅     | ✅   | ⚠️     | ⚠️     | ✅       | ⚠️   |
| Transaction Routes | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Operation Routes   | ✅   | ✅     | ✅   | ✅     | ✅     | ✅       | ⚠️   |
| Organizations      | ❌   | ❌     | ❌   | ❌     | ❌     | ❌       | ❌   |
| Users              | ❌   | ❌     | ❌   | ❌     | ❌     | ❌       | ❌   |
| Applications       | ❌   | ❌     | ❌   | ❌     | ❌     | ❌       | ❌   |

**Legend:**

- ✅ Fully tested
- ⚠️ Partially tested or not applicable
- ❌ Not tested (requires implementation)

---

**Document Version:** 1.0
**Last Updated:** October 6, 2025
**Maintainer:** Development Team
