# Web Console Frontend Guidelines

> Use this package for the React console under `web/console`.

---

## Scope

- `web/console/src`
- Vite / TypeScript / UI state / SSE client behavior

---

## Before You Change Code

1. Read `.trellis/spec/guides/index.md`
2. Check the matching backend event/types flow in:
   - `internal/api/http`
   - `internal/types`
3. Review current state handling in:
   - `web/console/src/api.ts`
   - `web/console/src/eventStream.ts`
   - `web/console/src/chatState.ts`

---

## Checklist

- Keep frontend payload assumptions aligned with backend event/type contracts
- Prefer minimal derived state; avoid duplicating server truth in multiple stores
- If event shapes change, update frontend tests in the same task

---

## Verification

```bash
cd web/console
npm.cmd run build
```
