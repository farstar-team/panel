# Troubleshooting

## Panel does not start

```bash
systemctl status farstar --no-pager
journalctl -u farstar -n 100 --no-pager
```

Check whether the configured panel port is already in use:

```bash
ss -lntp
```

## Panel is not reachable

The default bind address is localhost. Use an SSH tunnel:

```bash
ssh -L 8088:127.0.0.1:8088 root@SERVER_IP
```

If the panel intentionally binds to `0.0.0.0`, allow the panel port in the
server firewall and provider firewall. Exposing plain HTTP publicly is not
recommended.

## Tunnel remains in Starting

Open the tunnel log from the web interface or inspect:

```bash
ls -la /etc/farstar/logs
tail -n 100 /etc/farstar/logs/TUNNEL_ID.log
```

Common causes:

- tunnel port is already occupied
- server firewall blocks the tunnel port
- server and client secrets differ
- client remote address is incorrect
- WSS certificate name does not match the hostname
- local service is not listening

## Verify a local service

On the client server:

```bash
ss -lntp
nc -vz 127.0.0.1 LOCAL_PORT
```

## WSS certificate permission denied

The service runs as user `farstar`. Grant read access without making the key
world-readable. One option is a dedicated certificate group:

```bash
groupadd -f farstar-cert
usermod -aG farstar-cert farstar
chgrp farstar-cert /path/to/private-key.pem
chmod 0640 /path/to/private-key.pem
systemctl restart farstar
```

Apply equivalent permissions to parent directories.

## Reset a failed process state

Restart the panel:

```bash
systemctl restart farstar
```

At startup, Farstar reconciles stale process IDs with the database.

## Safe backup

Back up both files together:

```bash
systemctl stop farstar
tar -C /etc -czf farstar-backup.tar.gz farstar/farstar.db farstar/master.key
systemctl start farstar
```

Protect the resulting archive because it contains all material required to
decrypt tunnel secrets.
