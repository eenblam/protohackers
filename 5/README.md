# 5: Mob in the Middle

https://protohackers.com/problem/5

I think we can match all of this with a regex:

`^(?:.* )?(7[a-zA-Z0-9]{25,34})(?: .*)?$`

This takes advantage of:
* optional matching groups at the beginning and end
* range quantifiers to ensure the appropriate length (decremented by 1 on each end to account for the 7)


## Run
You can just do `go run .` to get the server running locally.

## Testing locally
Run tests with `go test -v .`

## Deploying to Digital Ocean
If you have [`doctl`](https://docs.digitalocean.com/reference/doctl/) set up locally,
you can just deploy with `./deploy.sh`.

You'll need to unlock your key, if passphrase protected.
The host key for your droplet will be automatically accepted,
so keep an eye out for that if you want to remove it yourself when done.

The script makes a best effort to clean up the droplet when done,
but you can confirm with `doctl compute droplet list | grep protohackers`.
