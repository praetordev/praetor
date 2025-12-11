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

# Drop privileges and exec
exec gosu praetor "$@"
