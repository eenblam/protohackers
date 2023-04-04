#!/bin/bash

NAME=protohackers-0
TARGET=smoketest
KEY=~/.ssh/id_rsa_do

# We don't want to catch Ctrl+C prior
handle_interrupt() {
	echo "Tearing down droplet ${NAME} with ID ${DROPLET}"
	doctl compute droplet delete -f $DROPLET
}
trap handle_interrupt SIGINT

# Just existing droplet if it wasn't cleaned up somehow,
# presumably by exiting too early
DROPLET=$(doctl compute droplet get $NAME --format=ID --no-header 2>/dev/null)
if [ -z "$DROPLET" ]; then
	if ! DROPLET=$(doctl compute droplet create \
		--region lon1 \
		--image debian-11-x64 \
		--size s-1vcpu-1gb \
		--ssh-keys 71:0b:6e:82:97:18:ef:cb:fc:27:85:ca:ce:14:bc:c3 \
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

# If we quit after this, delete the droplet
# We don't want to catch Ctrl+C prior
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


go build -o "$TARGET" main.go

echo "Copying binary"
# accept-new since we're just gonna TOFU the server's key
scp -i "$KEY" \
	-o StrictHostKeyChecking=accept-new \
	"./${TARGET}" "root@${IP}:/root/"


echo "Running binary. Ctrl+C to exit."
ssh -i "$KEY" "root@${IP}" "/root/${TARGET}"
