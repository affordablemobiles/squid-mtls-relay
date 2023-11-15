#!/bin/bash
set -e

rm -fr /var/log/squid/*
mkfifo /var/log/squid/access.log
mkfifo /var/log/squid/cache.log
chown -R proxy:proxy /var/log/squid

/sbin/process_fifo.sh /var/log/squid/access.log &
/sbin/process_fifo.sh /var/log/squid/cache.log &

echo "Starting squid..."
exec $(which squid) -f /etc/squid/squid.conf -NYCd 1
