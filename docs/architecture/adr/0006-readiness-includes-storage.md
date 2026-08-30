# ADR-0006: Readiness includes storage

- Status: Accepted

`/healthz` answers process liveness. `/readyz` is used for traffic admission
and must fail if the documented storage dependency cannot serve the tenant.
