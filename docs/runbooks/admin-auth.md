# Runbook: admin authentication

Confirm the admin uses its same-origin proxy/tunnel and calls `/go-json/auth/*`.
Check Codex cookie/security configuration, session-secret deployment, CSRF
response/header, and intended tenant before changing credentials. Rotate
secrets if exposure is suspected; do not enable broad credentialed CORS.
