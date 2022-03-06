#!/usr/bin/env sh

set -o errexit
set -o nounset

# Set default umask for created files inside the script
umask 077

# Check if sed is installed
command -v sed >/dev/null 2>&1 || { echo "sed must be installed for this script! Aborting"; exit 1; }
# check if date is installed
command -v date >/dev/null 2>&1 || { echo "date must be installed for this script! Aborting"; exit 1; }
# check if kubectl is installed
command -v kubectl >/dev/null 2>&1 || { echo "kubectl must be installed for this script! Aborting"; exit 1; }

cp ../sleepinfo.yaml /tmp/sleepinfo.yaml

unamestr=$(uname)

if [[ $unamestr == 'Darwin' ]]; then
  sed -i.bck "s|sleepAt: \"20:00\"|sleepAt: \"$(date -u -v+1M +"%H:%M")\"|g" "/tmp/sleepinfo.yaml"
  sed -i.bck "s|wakeUpAt: \"08:00\"|wakeUpAt: \"$(date -u -v+2M +"%H:%M")\"|g" "/tmp/sleepinfo.yaml"
else
  sed -i.bck "s|sleepAt: \"20:00\"|sleepAt: \"$(date -u -d '1 minutes' +"%H:%M")\"|g" "/tmp/sleepinfo.yaml"
  sed -i.bck "s|wakeUpAt: \"08:00\"|wakeUpAt: \"$(date -u -d '2 minutes' +"%H:%M")\"|g" "/tmp/sleepinfo.yaml"
fi

cat /tmp/sleepinfo.yaml

kubectl apply -f /tmp/sleepinfo.yaml -n $NAMESPACE
