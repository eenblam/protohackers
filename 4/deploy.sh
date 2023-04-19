#!/bin/bash

NAME=protohackers-4
TARGET=unusualdb
KEY=~/.ssh/id_rsa_do

# Just use existing droplet if it wasn't cleaned up somehow
# (presumably by exiting too early)
DROPLET=$(doctl compute droplet get $NAME --format=ID --no-header 2>/dev/null)
if [ -z "$DROPLET" ]; then
	if ! DROPLET=$(doctl compute droplet create \
		--region lon1 \
		--image debian-11-x64 \
		--size s-1vcpu-1gb \
		--ssh-keys c8:79:0b:65:47:36:b8:77:83:8e:97:cf:c5:3b:90:0b \
		$NAME \
		--format=ID \
		--no-header); then
		echo "Couldn't create droplet ${NAME}. Exiting"
		exit 1
	fi
	echo "Created droplet ${NAME} with ID ${DROPLET}"
else
	echo "Found droplet ${NAME} with ID ${DROPLET}"
fi

# If we quit after this, delete the droplet.
# Registering here, since we don't want to catch Ctrl+C prior.
handle_interrupt() {
	echo "Tearing down droplet ${NAME} with ID ${DROPLET}"
	doctl compute droplet delete -f $DROPLET
}
trap handle_interrupt SIGINT


echo "Waiting for droplet IP"
IP=$(doctl compute droplet get $DROPLET --format='PublicIPv4' --no-header 2>/dev/null)
while [ -z "$IP" ]; do
	sleep 5
	IP=$(doctl compute droplet get $DROPLET --format='PublicIPv4' --no-header)
done

echo "IP: ${IP}"

echo "Waiting for SSH up"
if ! nc -z $IP 22 ; then
	sleep 1
fi
# Can still have conn refused for a moment
sleep 2


go build -o "$TARGET" .

echo "Copying binary"
# accept-new since we're just gonna TOFU the server's key
scp -i "$KEY" \
	-o StrictHostKeyChecking=accept-new \
	"./${TARGET}" "root@${IP}:/root/"


echo "Running binary. Ctrl+C to exit."
ssh -i "$KEY" "root@${IP}" "/root/${TARGET}"
