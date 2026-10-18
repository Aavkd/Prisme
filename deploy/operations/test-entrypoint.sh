#!/bin/sh
set -eu
ssh-keygen -q -t ed25519 -N '' -f /fixture/client-key
ssh-keygen -q -t ed25519 -N '' -f /fixture/host-key
ssh-keygen -lf /fixture/host-key.pub | awk '{print $2}' > /fixture/host-fingerprint
mkdir -p /fixture/project/.ssh
cp /fixture/client-key.pub /fixture/project/.ssh/authorized_keys
chown -R prisme:prisme /fixture/project /fixture/coordination
chmod 700 /fixture/project/.ssh
chmod 600 /fixture/project/.ssh/authorized_keys
ln -s /fixture /fixture/project/escape
cat > /fixture/sshd_config <<'EOF'
Port 2222
ListenAddress 127.0.0.1
HostKey /fixture/host-key
AuthorizedKeysFile .ssh/authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
AllowUsers prisme
Subsystem sftp internal-sftp
LogLevel ERROR
EOF
/usr/sbin/sshd -f /fixture/sshd_config
exec /operations.test -test.v -test.timeout=90s
