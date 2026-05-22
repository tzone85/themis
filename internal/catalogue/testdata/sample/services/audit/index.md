---
id: audit
name: Audit
domain: Notifications
consumes:
  - NotificationSent
  - NotificationFailed
produces:
  - AuditExported
  - ReconciliationCompleted
---

The audit service produces auditor-ready exports.
