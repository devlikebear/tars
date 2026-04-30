# Frontend API Type Contracts

TARS Console currently keeps API response shapes in `frontend/console/src/lib/types.ts`
and request helpers in `frontend/console/src/lib/api.ts`.

For now, keep these types hand-curated instead of adding a broad OpenAPI or JSON
schema generation pipeline. The backend does not yet expose a single complete
schema source for every console endpoint, so a generator would add build
complexity without removing the manual contract boundary.

When a backend response shape changes, update the matching frontend type and add
focused validation in the same PR. For high-churn endpoints, prefer introducing a
small endpoint-specific schema source before adopting repository-wide generation.
