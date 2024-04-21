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

filename=$1

cp ./tests/e2e/$filename /tmp/$filename

unamestr=$(uname)

if [[ $unamestr == 'Darwin' ]]; then
  sed -i.bck "s|sleepAt: \"20:00\"|sleepAt: \"$(date -u -v-2M +"%H:%M")\"|g" "/tmp/$filename"
  sed -i.bck "s|wakeUpAt: \"08:00\"|wakeUpAt: \"$(date -u -v+1M +"%H:%M")\"|g" "/tmp/$filename"
else
  sed -i.bck "s|sleepAt: \"20:00\"|sleepAt: \"$(date -u -d '-2 minutes' +"%H:%M")\"|g" "/tmp/$filename"
  sed -i.bck "s|wakeUpAt: \"08:00\"|wakeUpAt: \"$(date -u -d '1 minutes' +"%H:%M")\"|g" "/tmp/$filename"
fi

cat /tmp/$filename

kubectl apply -f /tmp/$filename -n $NAMESPACE

rm /tmp/$filename
