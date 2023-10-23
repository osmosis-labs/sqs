#!/bin/bash
if [ "${DEBUG}" == "true" ]; then
  echo "keep node alive on failure for debugging" >> status
  echo "sleep for 30 seconds to let other services start"
  sleep 30
  /bin/sqsd || echo "failed to start" && tail -f status
else
  /bin/sqsd
fi