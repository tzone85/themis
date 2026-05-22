---
id: notifier
name: Notifier
domain: Notifications
consumes:
  - PaymentDispatched
produces:
  - NotificationSent
  - NotificationFailed
---

The notifier sends customer notifications on payment dispatch.
