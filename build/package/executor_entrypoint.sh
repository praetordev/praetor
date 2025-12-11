#!/bin/sh
set -e

# Copy keys if they exist in /tmp/keys
if [ -d "/tmp/keys" ]; then
    echo "Importing SSH keys from legacy mount or /tmp/keys..."
    
    # Handle private key
    if [ -f "/tmp/keys/id_rsa" ]; then
        cp /tmp/keys/id_rsa /home/praetor/.ssh/id_rsa
        chmod 600 /home/praetor/.ssh/id_rsa
    fi

    # Handle public key
    if [ -f "/tmp/keys/id_rsa.pub" ]; then
        cp /tmp/keys/id_rsa.pub /home/praetor/.ssh/id_rsa.pub
        chmod 644 /home/praetor/.ssh/id_rsa.pub
    fi
fi

# Ensure ownership is correct
chown -R praetor:praetor /home/praetor/.ssh

# Configure SSH to be permissive (Fixes invalid host key prompts)
cat <<EOF > /home/praetor/.ssh/config
Host *
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
EOF
chmod 600 /home/praetor/.ssh/config
chown praetor:praetor /home/praetor/.ssh/config

# Drop privileges and exec
exec gosu praetor "$@"
