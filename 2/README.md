# 2: Means to an End

https://protohackers.com/problem/2

Requirements:

* Accept TCP connections
* For each connection, provide an ephemeral data store.
* Receive 9-byte binary messages.
    * First (0-th) byte is a char indicating message type: `I` for Insert and `Q` for Query.
    * Bytes 1-4 and 5-8 are both int32s, with meaning dependent on type.
        * These are signed two's complement 32-bit ints in network byte order (big endian).
        * Insert: first is a unix timestamp, second is price of asset (in pennies) at that time.
            * Prices can be negative.
            * These are not strictly received in order.
            * Store the price for that timestamp.
        * Query: a range of time. The int32s are beginning and end timestamps (inclusive) for the range.
            * Provide the mean for the records within the time range.
            * Just write a single int32 in the same format

## Run
You can just do `go run .` to get the server running locally.

## Testing locally
Run tests with `go test -v .`

For rough integration testing, use two terminals:
* In terminal A, do `go run .`
* Then, in terminal B, do `cp test.go.bak test.go; go run test.go; rm test.go`
    * (Can't have two `main()` calls in the same package, and can't `go run test.go` unless in main package. :\ )

## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

You'll need to unlock your key, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
