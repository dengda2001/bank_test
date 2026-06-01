# Rent Management System V1 Design

## Purpose

Build a lightweight rent billing and reconciliation system for an Irish landlord managing about 300 rooms. The system should reduce manual bank-statement checking by generating monthly receivables, collecting payment transactions from multiple sources, matching payments to tenant bills, and showing the landlord the current billing status.

The commercial direction is "custom delivery first, productized architecture underneath." The first customer receives a tailored V1, while the technical model keeps room for future multi-landlord SaaS, paid modules, and provider switching.

## V1 Scope

V1 includes:

- Google login with invitation-based organization membership.
- Organization-level roles for owner, manager, and accountant.
- Property and room management.
- Tenant management with contact details and stable payment reference code.
- Lightweight contracts with start date, end date, monthly rent, deposit, payment day, room, tenant, status, and PDF attachment.
- Monthly receivable bill generation from contracts.
- Payment intake from one real Open Banking provider, with TrueLayer preferred and Tink as backup after coverage and pricing checks.
- Manual single payment entry.
- CSV/Excel bank statement import.
- System template import for historical payments and batch corrections.
- Payment normalization into one shared transaction pool.
- Automatic reconciliation for strong matches.
- Manual review for ambiguous or unidentified transactions.
- Lightweight bill management showing expected amount, paid amount, unpaid amount, partial payment, overdue status, pending review, and tenant bill history.
- Landlord dashboard for monthly expected, received, unpaid, overdue, partial, and pending review totals.
- Export of bill and reconciliation data.
- Audit logs for financial and security-sensitive actions.

V1 excludes:

- Full tenant portal. A lightweight tenant page may be added later.
- Manual or automatic rent reminders.
- Maintenance worker and work order management.
- Advanced analytics such as vacancy rate, turnover rate, rent trend analysis, property yield comparison, arrears rate, or tenant stability analysis.
- Full multi-landlord SaaS operations, although the data model must keep organization isolation.
- Online rent collection, payment initiation, SEPA Direct Debit, or payment processing.
- Complex contract rules such as rent increases, rent-free periods, utilities, multi-tenant splitting, and supplementary agreement versions.
- Full accounting functions such as invoices, tax reporting, general ledger, expense management, and bank balance management.

## Commercial Packaging

Recommended sales model:

- One-time V1 development fee: EUR 15,000 to EUR 25,000.
- Monthly maintenance: EUR 600 to EUR 1,000.
- Third-party costs such as Open Banking provider, hosting, email, SMS, WhatsApp, storage, and monitoring are charged separately or passed through with a management fee.

Suggested offer:

- Operational V1 Plus: EUR 22,000 development fee plus EUR 900 per month maintenance.
- Includes one real bank provider integration, payment imports, role-based access, audit trail, lightweight bill management, dashboard, and the first few months of operational tuning.

Future paid modules:

- Lightweight tenant portal.
- Manual and automatic rent reminders.
- Maintenance and work order management.
- Advanced operating analytics.
- Xero or QuickBooks export/integration.
- Complex contract rules.
- Platform SaaS administration for multiple landlords.

## Architecture

The system is organized into six modules:

- Organization & Access: organizations, users, memberships, roles, Google login, invitations, and audit logs.
- Property Registry: properties, rooms, tenants, and tenancy relationships.
- Contract & Receivables: lightweight contracts, contract term snapshots, monthly receivable bill generation, and bill state transitions.
- Payment Intake: Open Banking provider sync, CSV/Excel bank statement import, template import, and manual payment entry.
- Reconciliation: transaction normalization, duplicate detection, match suggestions, auto-allocation, manual confirmation, and allocation reversal.
- Billing Workspace: lightweight bill management, landlord dashboard, tenant bill history, filters, and export.

The core product boundary is not the bank API. The core is the shared payment transaction pool and reconciliation engine. Open Banking, bank file import, template import, and manual entry all feed the same normalized model.

## Core Data Flow

1. The owner signs in with Google, creates or accesses an organization, and invites assistants or accountants.
2. The team creates properties, rooms, tenants, and lightweight contracts.
3. Contracts generate monthly receivable bills using a contract term snapshot.
4. Payment transactions enter through Open Banking sync, CSV/Excel import, template import, or manual entry.
5. All payment sources are normalized into PaymentTransaction records.
6. The reconciliation engine checks tenant reference codes, amount, payment date window, payer information, account, and historical matches.
7. Strong matches are automatically allocated to receivable bills.
8. Ambiguous matches create review items with candidate bills and confidence information.
9. Unidentified payments remain in a processing pool until linked, ignored, or marked as non-rent.
10. Bill states update from allocations and drive the dashboard, bill workspace, tenant bill history, and exports.

## Account And Access Design

V1 uses Google login as the primary authentication method.

- Google proves the identity of the person.
- The local system decides which organization the person belongs to and what role they have.
- A user must be invited or approved before accessing organization data.
- Store Google's stable subject identifier and verified email.
- Use email for invitation matching, but do not rely on email alone as the permanent identity.
- Keep a local Membership record for organization role and permissions.
- Password login is out of V1 unless the customer explicitly requires it.

Roles:

- Owner: manage organization, members, bank connections, contracts, bills, allocations, exports, and settings.
- Manager: manage operational data, imports, manual payments, review queue, bills, tenants, rooms, and contracts within granted permissions.
- Accountant: view/export financial data, import statements, review allocations, and confirm reconciliation if permitted.

## Core Data Model

Primary entities:

- Organization: landlord business tenant for data isolation.
- User: authenticated person.
- Membership: user role inside one organization.
- Property: building or address grouping.
- Room: rentable unit linked to a property.
- Tenant: renter profile and payment reference code.
- Tenancy: active or historical occupancy relationship between tenant, room, and contract.
- Contract: lightweight contract source for receivable generation.
- ContractTermSnapshot: immutable billing terms used when generating bills.
- ReceivableInvoice: monthly bill with expected amount, paid amount, due date, and status.
- BankConnection: provider, authorization state, account identifier, masked account details, expiry, and last sync.
- ImportBatch: file or template import record with source, status, row results, and error details.
- PaymentTransaction: normalized incoming payment record from any source.
- MatchSuggestion: candidate allocation with confidence and matching rules.
- PaymentAllocation: confirmed allocation from a payment transaction to one or more bills.
- AuditLog: immutable record of sensitive business and security actions.

Bill states:

- draft
- open
- partially_paid
- paid
- overdue
- needs_review
- void

Payment transaction states:

- new
- auto_matched
- needs_review
- allocated
- ignored
- duplicate

## Contract Extensibility

V1 contracts stay lightweight, but the design must preserve future flexibility.

- Generated bills store contract term snapshots instead of reading mutable contract fields forever.
- Future changes affect future bills by default.
- Historical bills are not silently rewritten.
- Adjustments and reversals are recorded explicitly.
- Additional contract rule types can be introduced later while still producing standard ReceivableInvoice records.

Future contract extensions include rent increase schedules, rent-free periods, utility charges, service charges, multi-tenant splitting, and supplementary agreement versions.

## Payment Intake

Open Banking:

- V1 integrates one real provider.
- TrueLayer is the preferred first candidate.
- Tink is the main backup candidate.
- Final provider choice depends on the customer's exact bank, coverage, commercial terms, developer access, API behavior, and authorization flow.
- The system stores provider tokens encrypted and stores only necessary bank account metadata.

Manual and batch intake:

- Manual single payment entry supports date, amount, payer, reference, account, note, and optional tenant/bill hint.
- CSV/Excel bank statement import supports customer bank exports.
- System template import supports historical migration and structured batch payment entry.
- All import methods create ImportBatch records.
- Failed rows are visible with error reasons.
- Duplicate detection runs before reconciliation.

## Reconciliation Rules

The reconciliation engine uses confidence tiers:

- Auto-allocation: exact valid tenant reference code, amount is consistent with an open bill or clear partial allocation, payment date is within the expected window, and no duplicate or conflict exists.
- Manual review: missing or partial reference, unclear month, amount mismatch, multiple possible tenants, overpayment, underpayment, duplicate suspicion, or unusual payer information.
- Unidentified: no reliable tenant or bill candidate.

Recommended V1 reference format:

```text
RENT-{ROOM_CODE}-{TENANT_CODE}
```

Example:

```text
RENT-A12-T003
```

V1 does not require the tenant to include the billing month in the reference because payment date and open bill sequence usually provide enough context. A month suffix can be added later if needed:

```text
RENT-A12-T003-2026-06
```

Auto-allocation must still be reversible and audited.

## Lightweight Bill Management

The bill workspace is the main operating surface for V1.

It supports:

- Month, property, room, tenant, and status filters.
- Expected amount, paid amount, unpaid amount, due date, and status.
- Partial payment state.
- Overdue state.
- Pending review state.
- Tenant bill history.
- Payment allocation history.
- Export of visible bill data.

This is intentionally lighter than a full accounting module. It answers who should have paid, who has paid, who has partially paid, who is overdue, and which transactions still need review.

## Dashboard

The V1 landlord dashboard shows operational rent collection status:

- Total expected this month.
- Total received this month.
- Unpaid amount.
- Number of open bills.
- Number of partial bills.
- Number of overdue bills.
- Number of transactions needing review.
- Last bank sync time.
- Import errors needing attention.

Advanced analytics such as vacancy rate, turnover rate, rent trends, property yield comparison, and tenant stability are paid future modules.

## Security And Risk Controls

Security requirements are V1 acceptance criteria.

- All organization-owned records include organization_id and are scoped by organization in reads and writes.
- Users access data only through Membership permissions.
- Financial records cannot be silently overwritten after confirmation.
- Confirmed allocations can be reversed, not edited in place.
- Generated bills can be adjusted or voided with explicit records.
- Audit logs capture actor, timestamp, action, entity, original value, new value, and reason when applicable.
- Open Banking tokens are encrypted at rest.
- Bank account data is minimized to provider account id, masked account display, provider, authorization state, expiry, and sync metadata.
- Tenant contact data, contract attachments, and bank transaction descriptions are treated as sensitive data.
- HTTPS is required in production.
- Database backups run on a schedule.
- Attachment storage is backed up or versioned.
- Imports and bank syncs are idempotent.
- Duplicate detection prevents repeated allocation of the same transaction.
- Owner accounts should support MFA when available through Google.
- The system reads account information and performs reconciliation only. It does not initiate payments or collect rent.

Contractual boundary:

- The system assists reconciliation and record management.
- Final financial confirmation remains the customer's responsibility.
- Open Banking provider outages, authorization expiry, unsupported banks, and missing bank fields are external dependency conditions unless caused by implementation defects.

## Error Handling

Expected handling:

- Bank authorization expired: show reconnect action and last successful sync time.
- Provider does not support the bank: continue with CSV/Excel import and manual entry.
- Import format issue: reject invalid rows with actionable row-level errors.
- Duplicate transaction: mark as duplicate or needs review, never auto-allocate twice.
- Tenant underpayment: mark bill partially_paid and keep remaining amount open.
- Tenant overpayment: route to manual review in V1.
- Wrong tenant reference: route to manual review and store confirmed historical relationship after correction.
- Incorrect auto-allocation: allow reversal and record audit reason.
- Contract change: apply to future receivables; historical bills require explicit adjustment.

## Acceptance Criteria

V1 is accepted when:

- The landlord can maintain 300 rooms, tenants, contracts, and monthly receivable bills.
- A user can sign in with Google and access only organizations where they are a member.
- Owner can invite manager/accountant users and assign roles.
- Contracts generate monthly bills with correct expected amount and due dates.
- Payment transactions can enter from one real Open Banking provider.
- Payment transactions can also enter from manual single entry, bank statement import, and system template import.
- Imported and synced transactions are normalized into a shared transaction pool.
- Exact payment reference matches can auto-allocate without manual confirmation.
- Ambiguous payments enter a review queue.
- Manual review can confirm, ignore, or leave transactions unresolved.
- Allocations update bill states and tenant bill history.
- Confirmed allocations can be reversed with audit logging.
- The dashboard shows monthly expected, received, unpaid, overdue, partial, and pending review totals.
- The bill workspace shows tenant-level bill history and current bill status.
- Core data can be exported for record keeping or accountant review.
- Security controls, organization isolation, audit logs, and financial immutability rules are implemented.

Primary success metrics:

- Month-end reconciliation can be completed in minutes after bank sync or statement import.
- The system can operate the full set of 300 rooms without relying on spreadsheets as the source of truth.

Automatic match rate is an optimization metric, not a hard acceptance criterion, because tenant reference quality and bank field behavior are outside complete system control.

