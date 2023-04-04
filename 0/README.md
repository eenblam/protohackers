# 0: Smoke Test

https://protohackers.com/problem/0

Requirements:

* Accept TCP connection
* Queue data until EOF from client
* Reply with all data, then close the socket.
* Support at least 5 simultaneous connections

## Run

```
go run .
```

## Testing locally
In two terminals, you can run `go run .` to run the server, then `go run test.go` to run the tests.
