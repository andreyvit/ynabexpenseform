#!/bin/bash
set -euo pipefail

hostname="$1"
temp_file="$2"
password="$3"
username=$USER
port=3320

export DEBIAN_FRONTEND=noninteractive
# solves Perl complaining about invalid locales
export LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8

SUDO() { echo "# $@" >&2; sudo "$@"; }

# ===========================================================================

SUDO install -d -m755 -g $username -o $username /srv/ynabexpenseform/bin

SUDO install -m 755 -o root -g root ~/$temp_file /srv/ynabexpenseform/bin/ynabexpenseform

SUDO install -m644 -groot -oroot /dev/stdin /etc/systemd/system/ynabexpenseform.service <<EOF
[Unit]
Description=YNAB Expense Form
After=network.target
StartLimitIntervalSec=0

[Service]
User=$username
Restart=always
RestartSec=500ms
PIDFile=/run/ynabexpenseform.pid
Type=simple
ExecStart=/srv/ynabexpenseform/bin/ynabexpenseform -listen 127.0.0.1:$port
KillMode=process

[Install]
WantedBy=multi-user.target
EOF

SUDO install -m644 -groot -oroot /dev/stdin /srv/ynabexpenseform/Caddyfile <<EOF
$hostname {
    basic_auth * {
        assistant $password
    }
    reverse_proxy * http://127.0.0.1:$port {
        lb_try_duration 30s
        lb_try_interval 500ms
        transport http {
            dial_timeout 3s
            response_header_timeout 180s
            write_timeout 180s
            read_timeout 180s
            keepalive 2m
        }
    }
}
EOF

SUDO systemctl daemon-reload
SUDO systemctl enable ynabexpenseform
SUDO systemctl restart ynabexpenseform
/srv/caddy/bin/caddy reload --config /etc/Caddyfile
