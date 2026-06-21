# Security Policy

## Supported versions

Only the latest published release receives security updates.

## Reporting a vulnerability

Please use GitHub Private Vulnerability Reporting:

1. Open the repository Security tab.
2. Select **Report a vulnerability**.
3. Include affected version, reproduction steps, impact, and suggested fix.

Do not publish credentials, tunnel secrets, database files, master keys, server
addresses, or working exploit details in a public issue.

## Operational recommendations

- Keep the panel bound to `127.0.0.1` unless HTTPS is configured.
- Use a long, unique admin password.
- Keep `/etc/farstar/master.key` readable only by the `farstar` user.
- Keep the operating system and Farstar release updated.
- Never enable Skip TLS Verify for permanent WSS tunnels.
