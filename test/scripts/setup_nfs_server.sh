#!/bin/bash

set -e 

NFS_DIR="/nfs-data"
LOCAL_IPS="172.16.0.0/12"

if ! command -v exportfs &> /dev/null; then
    echo "Installing NFS server..."
    sudo apt update && sudo apt install -y nfs-server
else
    echo "NFS server is already installed."
fi

if [[ ! -d "$NFS_DIR" ]]; then
    echo "Creating $NFS_DIR directory..."
    sudo mkdir -p "$NFS_DIR"
    sudo chmod 777 "$NFS_DIR"
fi

EXPORTS_ENTRY="$NFS_DIR $LOCAL_IPS(rw,sync,no_subtree_check,no_root_squash)"
if ! grep -q "$NFS_DIR" /etc/exports; then
    echo "Adding NFS export rule: $EXPORTS_ENTRY"
    echo "$EXPORTS_ENTRY" | sudo tee -a /etc/exports
else
    echo "NFS export rule already exists."
fi

echo "Restarting NFS server..."
sudo exportfs -arv 
sudo systemctl restart nfs-server 
sudo systemctl enable --now nfs-server

echo "NFS server is set up and running!"

