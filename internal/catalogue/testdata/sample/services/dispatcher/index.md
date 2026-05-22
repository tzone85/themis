---
id: dispatcher
name: Dispatcher
domain: Collections
consumes:
  - PaymentReceived
produces:
  - PaymentDispatched
---

The dispatcher routes payments to the correct downstream handler.
