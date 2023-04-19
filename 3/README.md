# 3: Budget Chat

https://protohackers.com/problem/3

The requirements on this one are simpler than #2, so I won't elaborate on them in the same way.

The trick to this challenge is

1. not deadlocking
1. not blocking in a way that deadlocks
    * My initial idea *could* deadlock if the Broker wanted to notify B that A left, but A was itself blocking on Logoff instead of listening to a channel.
1. not buffering in a way that can be overwhelmed (Go's buffered channel could have "solved" the above issue for moderate traffic, but could eventually be forced to block again with sufficient load.)

## Run
You can just do `go run .` to get the server running locally.

## Testing locally
No tests yet.

## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

You'll need to unlock your key, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
