# Runbook: failed release

Freeze promotion, preserve logs and release pin/manifest details, return traffic
only to the prior compatible binary, and verify readiness/schema/auth/media.
Do not downgrade storage or restore tenant data unless corruption is confirmed.
Capture the failed gate and amend release evidence before retrying.
