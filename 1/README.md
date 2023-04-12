# 1: Primetime

https://protohackers.com/problem/1

Requirements:

* Accept TCP connections
* For each connection, read one line at at time.
* Attempt to parse each line as JSON.
* Validate each request is well-formed.
    * Respond to malformed requests with a malformed response, then close connection.
* For valid requests, respond with a response indicating if the requested number is prime.
* Support further requests until connection is closed by the client or a malformed request is received.

## Run
You can just do `go run .` to get the server running locally.

## Testing locally
In two terminals, you can run `go run .` to run the server. No tests at present.


## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

You'll need to unlock your key, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
