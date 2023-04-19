# 4: Unusual Database Program

https://protohackers.com/problem/4

Pretty straightforward.
I started writing parser tests in advance,
then realized that my parse function had the
exact same signature as `strings.Cut()`.
This made life pretty nice.

This passes, but I suspect the tests
didn't put much concurrent load on the system.

## Run
You can just do `go run .` to get the server running locally.

## Testing locally
No tests right now.

## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

You'll need to unlock your key, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
