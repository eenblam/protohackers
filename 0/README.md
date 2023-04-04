# 0: Smoke Test

https://protohackers.com/problem/0

Requirements:

* Accept TCP connection
* Queue data until EOF from client
* Reply with all data, then close the socket.
* Support at least 5 simultaneous connections

## Run
You can just do `go run .` to get the server running locally.

## Testing locally
In two terminals, you can run `go run .` to run the server, then `go run test.go` to run the tests.


## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

You'll need to unlock your key, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
