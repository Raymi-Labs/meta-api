#!/bin/sh
# Reload only after renewal of this documentation site's certificate.
set -eu
if [ "${RENEWED_LINEAGE:-}" = /etc/letsencrypt/live/docs.meta-api.vip ] ||
   [ "${RENEWED_LINEAGE:-}" = /etc/letsencrypt/live/docs.meta-api.com ]; then
    nginx -t
    systemctl reload nginx
fi
