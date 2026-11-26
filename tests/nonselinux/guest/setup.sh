#!/usr/bin/env bash
set -euo pipefail

log() { printf '[guest-setup] %s\n' "$*" >&2; }

as_dev() {
  runuser -u devuser -- env -i HOME=/home/devuser USER=devuser LOGNAME=devuser PATH=/usr/bin:/bin XDG_RUNTIME_DIR=/run/user/1002 bash -c "cd /home/devuser && $1"
}

# No SELinux packages: this guest boots with selinux=0.
dnf install -y --setopt=install_weak_deps=False \
  podman crun fuse-overlayfs slirp4netns shadow-utils

modprobe fuse || true
[[ -c /dev/fuse ]] || mknod /dev/fuse c 10 229

if [[ -d /sys/fs/selinux ]] || [[ "$(getenforce 2>/dev/null || echo Disabled)" != "Disabled" ]]; then
  log "expected SELinux disabled, got: $(getenforce 2>/dev/null || echo '?')"
  exit 1
fi
{ getenforce 2>/dev/null || echo Disabled; } > /root/getenforce.txt

groupadd -g 1006 devuser || true
useradd -u 1002 -g 1006 -m -s /bin/bash devuser || true
printf 'devuser:100000:65536\n' > /etc/subuid
printf 'devuser:100000:65536\n' > /etc/subgid

printf 'd /run/user/1002 0700 1002 1006 -\n' > /etc/tmpfiles.d/pinp-devuser.conf
systemd-tmpfiles --create /etc/tmpfiles.d/pinp-devuser.conf

mkdir -p /home/devuser/workdir/build /home/devuser/bin /home/devuser/image /home/devuser/.config/containers
printf '[engine]\ncgroup_manager = "cgroupfs"\nevents_logger="file"\nruntime="crun"\n' > /home/devuser/.config/containers/containers.conf
cp -r /root/pinp-image/. /home/devuser/image/
cp /root/pinp-bin/pinp-e /root/pinp-bin/pinp-i /home/devuser/bin/
printf 'FROM docker.io/library/debian:trixie\nRUN echo ok\n' > /home/devuser/workdir/build/Containerfile
chown -R 1002:1006 /home/devuser
chmod +x /home/devuser/bin/pinp-e /home/devuser/bin/pinp-i

as_dev 'mkdir -p ~/.config/containers ~/.local/share/containers && podman info > /dev/null'
as_dev 'podman pull docker.io/library/debian:trixie'
as_dev 'podman build -t pinp-outer /home/devuser/image'
as_dev 'podman image ls'
as_dev 'podman info --format "{{.Host.Security.SELinuxEnabled}}"'
as_dev 'PINP_EXPLAIN=1 /home/devuser/bin/pinp-e run debian:trixie'
