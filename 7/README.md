# 7: Line Reversal

https://protohackers.com/problem/7

The challenge: implement the specified lightweight transport layer
(basically TCP-over-UDP) and then implement a simple line-reversal
server on top of it.

My transport layer looks just like a standard TCP server in Go:

* Create a new `Listener` with `Listen`, then handle new connections with `listener.Accept()`.
* Accepted connections will create a `Session`, which implements the usual [`ReadWriteCloser`](https://pkg.go.dev/io#ReadWriteCloser) interface.
    * Connection state is managed in `session.go`.
* Proceed as you would with a standard TCP server.

## Run
You can just do `go run .` to get the server running locally.

## Testing locally
`go test -v -cover`

## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

Note that you'll need to update the key and fingerprint in the script,
and you'll need to unlock your key on run, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
