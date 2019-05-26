# Hosting Based Interface

## The Problem

### Inefficient Implementations of Request-Response Pattern

Most classical implementations of the classic **Request-Response** pattern are synchronous,
`HTTP/1.x`, for the most typical example, is to pend subsequent out-bound transportation
through the underlying transport, wait until the expected response has been sent back from
peer endpoint, as in-bound transportation through the underlying transport, before
the next request is let-go to start off.

The result is that transports (tcp connections for HTTP), wasted too much time in
[Round-trip Delay Time](https://en.wikipedia.org/wiki/Round-trip_delay_time).
That's why **MANY** (yet **SMALL**) js/css files must be packed into **FEW** (yet **LARGE**)
ones for decent page loading time, while the total traffic amount is almost the same.

[HTTP Pipelining](https://en.wikipedia.org/wiki/HTTP_pipelining)
has helped benchmarks to reach
[A Million requests per second](https://medium.freecodecamp.org/million-requests-per-second-with-python-95c137af319),
but that's [not helping for realworld cases](https://devcentral.f5.com/s/articles/http-pipelining-a-security-risk-without-real-performance-benefits).

Newer protocols like `HTTP/2` and `HTTP/3` (a.k.a
[HTTP-over-](https://www.zdnet.com/article/http-over-quic-to-be-renamed-http3/)
[QUIC](https://en.wikipedia.org/wiki/QUIC)
)
are addressing various issues,
especially including the above said ones, but suffering from legacy burden for backward
compatibility with `HTTP/1.1`, they have gone not far.

### One Service On Behalf of Many

A human user thinks, between actions he/she will take to perform, and the thinking
usually takes previous action's result into account. Waiting for result/response is
a rather natural being in single user scenarios.

While most decent computer application systems comprise far more than a single service,
and even a lone service, are rather likely to be integrated with other systems/services
into larger solutions, to be useful today.

Such a **service** software typically act on behalf of many users concurrently, with main
line scenarios involve coordinated operations with other **service**s in reaction to
each user's each activity. This is largely different from a traditional **client**
software which assumes single user's activities only. If every user activity is to be
waited, there will be just too much waitings.

## The Solution

**Hosting Based Interface** - HBI implements asynchronous **Request-Response** pattern
under the hood. Building new services and applications with **HBI**, you can take full
advantage of modern concurrency mechanisms like
[Goroutines](https://tour.golang.org/concurrency/1) in Golang and
[asyncio](https://docs.python.org/3/library/asyncio.html) in Python, to have the classic
**Request-Response** pattern go naturally & very efficiently without imposing the dreadful
[RTT](https://en.wikipedia.org/wiki/Round-trip_delay_time)
and
[HOL blocking](https://en.wikipedia.org/wiki/Head-of-line_blocking).
Note: Over TCP connections, HBI eliminates HOL at _request/response_ level, to further
eliminate HOL at transport level, [QUIC](https://en.wikipedia.org/wiki/QUIC) will be needed.

**HBI** also eanbles **scripting** capability for both **request** and **response** bodies,
so both service authors and consumers can have greater flexibility in implementing
heterogeneous, diversified components speaking a same set of API/Protocol.

### In Action

Checkout [HBI Chat](https://github.com/complyue/hbichat), start a server, start several clients,
then start spamming the server from most clients, with many bots and many file uploads/downloads.
Leave 1 or 2 clients observing one or another spammed room for fun.

But better run it on a RamDisk, i.e. cd to `/dev/shm` on systems providing it like Linux, or
[create one on macOS](https://apple.stackexchange.com/questions/298836/create-an-apfs-ram-disk).
Spinning disks will bottleneck your stress, and you don't want to sacrifice your SSD's
lifespan just to upload/download random data files.

That project can be considered an [SSCCE](http://www.sscce.org/) HBI application.

## What is HBI

### API Defined Protocol

**HBI** is a _meta protocol_ for application systems (read **service** software components),
possibly implemented in different programming languages and/or base runtimes,
to establish communication channels between
[os process](<https://en.wikipedia.org/wiki/Process_(computing)>)es
(may or may not across computing nodes), as to communicate with _peer-scripting-code_ posted
to eachother's
**hosting environment**.

By providing a **hosting environment** which exposes necessary artifacts (various
**functions** in essense, see Mechanism) to accommodate the **landing** of the
_peer-scripting-code_ from the other end, a service process defines both its
[API](https://en.wikipedia.org/wiki/Application_programming_interface) and the effect
network protocol to access the API, at granted efficiency.

Such network protocols are called **API Defined Protocol**s.

### First Class Server Push

Server Push is intrinsic to API authors and consumers in defining and consuming the service
respectively, when the application protocol is implemented with **HBI**.

What should be available to the back-script, and what will be triggered at the consuming
site in response to each posting conversation it sends, is at the API authors' discretion.
While the reacting behavior at the consuming site as it's back-scripted, is at the consumer's
discretion.

And the service site can schedule posting conversations to be started against any connected
consumer endpoint, at appropriate time, to push system events in realtime.

### Throughput Oriented Communication Infrastructure

By [Pipelining](<https://en.wikipedia.org/wiki/Pipeline_(computing)>) the underlying transport
wire, though lantency of each single API call is not improved, but overall, the system can process
largely more communications in a given window of time, i.e. optimal efficiency at throughput.

And **HBI** specifically supports responses returned in different orders than their respective
requests were sent, so as to break [HOL blocking](https://en.wikipedia.org/wiki/Head-of-line_blocking).
Without relief from HOL blocking, solutions like
[HTTP Pipelining](https://en.wikipedia.org/wiki/HTTP_pipelining) is not on a par, see
[HTTP Pipelining: A security risk without real performance benefits](https://devcentral.f5.com/s/articles/http-pipelining-a-security-risk-without-real-performance-benefits).

### Example - Download a File in a Room

You should see:

- There's no explicit network manipulations, just send/recv using the converstion object:
  - `co.start_send()` / `co.start_recv()`
  - `co.send_obj()` / `co.recv_obj()`
  - `co.send_data()` / `co.recv_data()`
- Control info (i.e. request, accept/refuse msg, file size etc.) comprise of just function arguments
  and received value objects
- Binary data streaming is straight forward, no excessive memory used to hold full
  file data, and the buffer size can be arbitrarily chosen at either side
- The checksum of full data stream is calculated straight forward as well, no extra load or scan

And you should know:

- All network traffic is pipelined with a single tcp connection underlying
- The underlying tcp connection is shared with other service calls
- No service call will pend the tcp connection to block other calls, when it's not sending sth
- So the tcp connection is always at its max throughput potential (with neither RTT nor HOL blocking)
- No sophiscated network protocol design & optimisation needed to achieve all above

Service API Implementation:

```python
    async def SendFile(self, room_id: str, fn: str):
        co: HoCo = self.ho.co()
        # transit the hosting conversation to `send` stage a.s.a.p.
        await co.start_send()

        fpth = os.path.abspath(os.path.join("chat-server-files", room_id, fn))
        if not os.path.exists(fpth) or not os.path.isfile(fpth):
            # send negative file size, meaning download refused
            await co.send_obj(repr([-1, f"no such file"]))
            return

        s = os.stat(fpth)

        with open(fpth, "rb") as f:
            # get file data size
            f.seek(0, 2)
            fsz = f.tell()

            # send [file-size, msg] to peer, telling it the data size to receive and last
            # modification time of the file.
            msg = "last modified: " + datetime.fromtimestamp(s.st_mtime).strftime(
                "%F %T"
            )
            await co.send_obj(repr([fsz, msg]))

            # prepare to send file data from beginning, calculate checksum by the way
            f.seek(0, 0)
            chksum = 0

            def stream_file_data():  # a generator function is ideal for binary data streaming
                nonlocal chksum  # this is needed outer side, write to that var

                # nothing prevents the file from growing as we're sending, we only send
                # as much as glanced above, so count remaining bytes down,
                # send one 1-KB-chunk at max at a time.
                bytes_remain = fsz
                while bytes_remain > 0:
                    chunk = f.read(min(1024, bytes_remain))
                    assert len(chunk) > 0, "file shrunk !?!"
                    bytes_remain -= len(chunk)

                    yield chunk  # yield it so as to be streamed to client
                    chksum = crc32(chunk, chksum)  # update chksum

                assert bytes_remain == 0, "?!"

            # stream file data to consumer end
            await co.send_data(stream_file_data())

        # send chksum at last
        await co.send_obj(repr(chksum))
```

Consumer Usage:

```python
    async def _download_file(self, room_id: str, fn: str):
        room_dir = os.path.abspath(f"chat-client-files/{room_id}")
        if not os.path.isdir(room_dir):
            print(f"Making room dir [{room_dir}] ...")
            os.makedirs(room_dir, exist_ok=True)

        async with self.po.co() as co:  # start a new posting conversation

            # send out download request
            await co.send_code(
                rf"""
SendFile({room_id!r}, {fn!r})
"""
            )

            # transit the conversation to `recv` stage a.s.a.p.
            await co.start_recv()

            fsz, msg = await co.recv_obj()
            if fsz < 0:
                print(f"Server refused file downlaod: {msg}")
                return

            if msg is not None:
                print(f"@@ Server: {msg}")

            fpth = os.path.join(room_dir, fn)

            # no truncate in case another spammer is racing to upload the same file.
            # concurrent reading and writing to a same file is wrong in most but this spamming case.
            f = os.fdopen(os.open(fpth, os.O_RDWR | os.O_CREAT), "rb+")
            try:
                total_kb = int(math.ceil(fsz / 1024))
                print(f" Start downloading {total_kb} KB data ...")

                # prepare to recv file data from beginning, calculate checksum by the way
                chksum = 0

                def stream_file_data():  # a generator function is ideal for binary data streaming
                    nonlocal chksum  # this is needed outer side, write to that var

                    # receive 1 KB at most at a time
                    buf = bytearray(1024)

                    bytes_remain = fsz
                    while bytes_remain > 0:

                        if len(buf) > bytes_remain:
                            buf = buf[:bytes_remain]

                        yield buf  # yield it so as to be streamed from client

                        f.write(buf)  # write received data to file

                        bytes_remain -= len(buf)

                        chksum = crc32(buf, chksum)  # update chksum

                        remain_kb = int(math.ceil(bytes_remain / 1024))
                        print(  # overwrite line above prompt
                            f"\x1B[1A\r\x1B[0K {remain_kb:12d} of {total_kb:12d} KB remaining ..."
                        )

                    assert bytes_remain == 0, "?!"

                    # overwrite line above prompt
                    print(f"\x1B[1A\r\x1B[0K All {total_kb} KB received.")

                # receive data stream from server
                start_time = time.monotonic()
                await co.recv_data(stream_file_data())
            finally:
                f.close()

            peer_chksum = await co.recv_obj()

        elapsed_seconds = time.monotonic() - start_time

        print(  # overwrite line above
            f"\x1B[1A\r\x1B[0K All {total_kb} KB downloaded in {elapsed_seconds:0.2f} second(s)."
        )
        # validate chksum calculated at peer side as it had all data sent
        if peer_chksum != chksum:
            print(f"But checksum mismatch !?!")
        else:
            print(
                rf"""
@@ downloaded {chksum:x} [{fn}]
"""
            )
```

### Mechanism

An `HBI` communication channel (wire) works Peer-to-Peer, each peer has 2 endpoints:
the `posting endpoint` and the `hosting endpoint`. At any time, either peer can start a
`posting conversation` from its `posting endpoint` for active communication; and once the
other peer sees an incoming conversation at its `hosting endpoint`, it triggers a
`hosting conversation` for passive communication.

A `posting conversation` is created by the application. It starts out in `send` stage, in
which state the application can send with it any number of textual **peer-script** packets,
optionally followed by binary data/stream; then the application transit it to `recv` stage,
in which state the **response** generated at peer site by _landing_ those scripts will be
received with it at the originating peer. In case of **fire-and-forget** style notification
sending, the application just closes the `posting conversation` after all sent out.

A `hosting conversation` is created by HBI, and automatically available to the application
from the `hosting endpoint`. It starts out in `recv` stage, in which state the application
can recveive with it a number of value objects and/or data/streams specified by API design.
The application transits the `hosting conversation` to `send` stage if it quickly has the
response content to send back, or it can first transits the `hosting conversation` to `work`
stage to fully release the underlying HBI wire, before some time is spent to prepare the
response (computation or coordination with other resources); then finally transits to
`send` stage to send the response back. In case of **fire-and-forget** style communication,
no reponse is needed and the `hosting conversation` is closed once full request body has
been received with it.

There is a `hosting environment` attached to each `hosting endpoint`, the application
exposes various artifacts to the `hosting environment`, meant to be scripted by the
**peer-script** sent from the remote peer. This is why the whole mechanism called
_Hosting Based Interface_.

Hosted execution of **peer-script** is called **landing**, it is just interpeted running of
textual scripting code, with the chosen programming language / runtime, with the
`hosting envrionment` as context. e.g.
[exec()](https://docs.python.org/3/library/functions.html#exec) is used with Python, and
[Anko interpreter](https://github.com/mattn/anko "Anko's Github Home") is used with Golang.

The `hosting environment` of one peer is openly accessible by **peer-script**
from another peer, to the extent the hosting peer is willing of exposure.

The **peer-script** can carry data sending semantics as well as transactional semantics,
its execution result can be received by the `hosting conversation` as a value object, or
the following binary data/stream can be extracted from the wire and pushed to the
`hosting environment`.

There normally occur subsequences as the hosting peer is being scripted to do anything,
e.g. in case the posting peer is a software agent in behalf of its user to start a live
video casting, all the user's subscribers should be notified of the starting of video
stream, and a streaming channel should be established to each ready subscriber, then the
broadcaster should be notified how many subscribers will be watching.

The **peer-script** instructs about all those things as **WHAT** to do, and the
`hosting envirnoment` should expose enough artifacts implementing **HOW** to do each of those.

Theoretically every artifact exposed by the hosting environment is a **function**, which takes
specific number/type of arguments, generates side-effects, and returns specific number/type
of result (no return in case the number is zero).

While with Object-Oriented programming paradigm, there arose more types of **function** s that
carrying special semantics:

- `constructor` function:

  that creates a new (tho not strictly necessary newly allocated) object on each call

  Note:
  In HBI paradigm the `new` keyword should not appear for invocation of
  a `ctor` function. HBI peer script follows Python syntax and is different
  from Go/C++/Java/JavaScript syntax.

- `reactor method` function:

  that has a `reactor` object bound to it

  Meaning it has an implicit argument referencing the `reactor object`, in addition to its
  formal arguments.

The implementation of a **function** exposed by a `hosting environment`, normally (the exact
case that `response` is expected) does leverage the `hosting conversation` to send another
set of **peer-script** packets, optionally with binary data/stream (the `response`), back to
the original peer, for the subsequences be realized at the initiating site.

This set of **peer-script** packets (possibly followed by binary data/stream), is `landed`
(meaning received & processed) during the `recv` stage of the original peer's
`posting conversation`, as mentioned in earlier paragrah of this section.

Additionally, the implementation can schedule more activities to happen later, and any
activity can then start new `posting conversation`s to the _OP_, i.e. communication in the
reverse direction.

Orchestration forms when multiple service/consumer nodes keep communicating with many others
through p2p connections.

### HBI over vanilla TCP

- Python 3.7+ Client

```python
import asyncio, hbi


async def say_hello_to(addr):
    po, ho = await hbi.dial_socket(addr, hbi.HostingEnv())
    async with po.co() as co:
        await co.send_code(
            f"""
my_name = 'Nick'
hello()
"""
        )
        await co.start_recv()
        msg_back = await co.recv_obj()
    print(msg_back)
    await ho.disconnect()


asyncio.run(say_hello_to({"host": "127.0.0.1", "port": 3232}))
```

- Output

```console
cyue@cyuembpx:~$ python -m hbichat.cmd.hello.client
Welcome to HBI world!
Hello, Nick from 127.0.0.1:51676!
cyue@cyuembpx:~$
```

- Python 3.7+ Server

```python
import asyncio, hbi


def he_factory() -> hbi.HostingEnv:
    he = hbi.HostingEnv()

    he.expose_function(
        "__hbi_init__",  # callback on wire connected
        lambda po, ho: po.notif(
            f"""
print("Welcome to HBI world!")
"""
        ),
    )

    async def hello():
        co = he.ho.co()
        await co.start_send()
        consumer_name = he.get("my_name")
        await co.send_obj(repr(f"Hello, {consumer_name} from {he.po.remote_addr}!"))

    he.expose_function("hello", hello)

    return he


async def serve_hello():

    server = await hbi.serve_socket(
        {"host": "127.0.0.1", "port": 3232},  # listen address
        he_factory,  # factory for hosting environment
    )
    print("hello server listening:", server.sockets[0].getsockname())
    await server.wait_closed()


try:
    asyncio.run(serve_hello())
except KeyboardInterrupt:
    pass
```

- Output

```console
cyue@cyuembpx:~$ python -m hbichat.cmd.hello.server
hello server listening: ('127.0.0.1', 3232)
```

- Go1 Client

```go
package main

import (
	"fmt"

	"github.com/complyue/hbi"
)

func main() {
	he := hbi.NewHostingEnv()

	he.ExposeFunction("print", fmt.Println)

	po, ho, err := hbi.DialTCP("localhost:3232", he)
	if err != nil {
		panic(err)
	}
	defer ho.Close()

	co, err := po.NewCo()
	if err != nil {
		panic(err)
	}
	defer co.Close()

	if err = co.SendCode(`
my_name = "Nick"
hello()
`); err != nil {
		panic(err)
	}

	co.StartRecv()

	msgBack, err := co.RecvObj()
	if err != nil {
		panic(err)
	}
	fmt.Println(msgBack)
}
```

- Output

```console
cyue@cyuembpx:~$ go run github.com/complyue/hbichat/cmd/hello/client
Welcome to HBI world!
Hello, Nick from 127.0.0.1:51732!
cyue@cyuembpx:~$
```

- Go1 Server

```go
package main

import (
	"fmt"
	"net"

	"github.com/complyue/hbi"
)

func main() {
	hbi.ServeTCP("localhost:3232", func() *hbi.HostingEnv {
		he := hbi.NewHostingEnv()

		he.ExposeFunction("__hbi_init__", // callback on wire connected
			func(po *hbi.PostingEnd, ho *hbi.HostingEnd) {
				po.Notif(`
print("Welcome to HBI world!")
`)
			})

		he.ExposeFunction("hello", func() {
			co := he.Ho().Co()
			if err := co.StartSend(); err != nil {
				panic(err)
			}
			consumerName := he.Get("my_name")
			if err := co.SendObj(hbi.Repr(fmt.Sprintf(
				`Hello, %s from %s!`,
				consumerName, he.Po().RemoteAddr(),
			))); err != nil {
				panic(err)
			}
		})
		return he
	}, func(listener *net.TCPListener) {
		fmt.Println("hello server listening:", listener.Addr())
	})
}
```

- Output

```console
cyue@cyuembpx:~$ go run github.com/complyue/hbichat/cmd/hello/server
hello server listening: 127.0.0.1:3232
```

- ES6

```js
// Coming later, if not sooner ...
```

### HBI over [QUIC](https://en.wikipedia.org/wiki/QUIC)

Concurrent conversations can work upon QUIC streams, coming later, if not sooner ...
