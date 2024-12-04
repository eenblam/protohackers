# 7: Line Reversal

https://protohackers.com/problem/7

The challenge: implement the specified lightweight transport layer (which looks a lot like TCP-over-UDP) dubbed LRCP and then implement a simple line-reversal server application on top of it.

My transport layer interface matches (most) of that of a standard TCP server in Go:

* Create a new `Listener` with `Listen`, then handle new connections with `listener.Accept()`.
* Accepted connections will create a `Session`, which implements the usual [`ReadWriteCloser`](https://pkg.go.dev/io#ReadWriteCloser) interface.
    * This means a `Session` can also play nicely with things like `bufio.Scanner`.
* Proceed as you would with a standard Go TCP connection.

## Data flow
There are four message types to the protocol: `connect`, `close`, and `ack` for control, and `data` for transmission. Here's the flow for data messages:

* `Session.Write(data)` writes to a buffer.
* `Session.writeWorker()` goroutine checks this buffer regularly for updates, then encodes data into a `Msg`, which is sent to the peer via `Session.sendData(msg)`.
* The receiving listener (`Listener.listen()` and `Session.listenClient()` goroutines for server and client, respectively) reads a UDP datagram, parses a `Msg`, and forwards the message to a `Session.readWorker()` goroutine via a channel based on the session ID.
* `Session.readWorker()` handles the message; for data messages, it copies the data to a read buffer via `Session.appendRead(msg.Pos, msg.Data)`. Regardless of whether or not the data is able to be added to the buffer, it acks the most recently successful message and signals a read is available via a channel.
    * This is similar to what you might expect from a `sync.Cond`, but feels more straightforward.
* Whenever the read channel is signaled, `Session.Read(buf)` is unblocked and able to read from the read buffer.

Additional machinery is in place to handle things like retransmission of un-acked packets.

## Run
You can just do `go run .` to get the server running locally, or `go build . && lrcp`.

## Testing locally
`go test -v -cover .`

There are a few parsing-related unit tests, along with an integration test for sending a large amount of random data over an unreliable UDP proxy.

## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally, you can just deploy with `./deploy.sh`.

Note that you'll need to update the key and fingerprint in the script,
and you'll need to unlock your key on run, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
