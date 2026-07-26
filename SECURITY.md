# Security Policy

## Reporting a vulnerability

Please do not disclose security vulnerabilities in a public issue. Use GitHub's private vulnerability reporting feature for this repository instead.

Include the affected version or commit, reproduction steps, impact, and any suggested mitigation. Do not include real Canvas access tokens, student data, or institution-private URLs in the report.

## Credential handling

Canvas CLI reads access tokens from `CANVAS_API_TOKEN` and never intentionally writes them to its configuration file. Treat a Canvas access token like a password:

- grant only the scopes required for the task;
- never place it in command arguments, request URLs, logs, fixtures, or commits;
- revoke and replace it immediately if exposure is suspected.

The client sends this bearer token only to the configured Canvas origin. If a
request or download redirects to a different host, port, or URL scheme, the
authorization header is removed before following the redirect.

## Supported versions

Until tagged releases are available, security fixes target the latest commit on `main`.
