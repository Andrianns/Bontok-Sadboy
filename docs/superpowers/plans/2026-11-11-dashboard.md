# Bontok Sadboy UI Implementation Plan

Goal: Implement the approved React dashboard using shadcn, Indonesian UI, demo news and local drafts.
Architecture: React SPA with local persisted state. Providers and publishing are explicit simulations.
Tech Stack: React, TypeScript, Vite, Tailwind, shadcn Radix.
Spec: Approved dashboard design in this chat.

- [x] Bootstrap app and shadcn components. Verify build.
- [x] Build sidebar, Today and news discovery with filters and details.
- [x] Implement persisted drafts, editable script, timed 60-second preview, render and approval states.
- [x] Implement per-platform publishing simulation and retry without duplicating successful uploads.
- [x] Implement model, Higgsfield, news and account settings with demo connections.
- [x] Run production build and inspect responsive layout and normal workflow.

Decisions: native execution already authorized by user; use demo visual preview with clearly labeled simulated rendering. Store only non-secret preferences locally. Never persist entered API keys.


Verification: production build passed; browser workflow passed including retry, persistence, no page errors and no mobile overflow. Final review fixes: invalidate approval at render start, close mobile navigation on page change, bounded subtitle segments.
