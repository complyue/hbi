# Hosting Based Interface

## Purpose

### request/response pattern for RPC/IPC and other Inter-Service Comm

    A classical implementation of the classic `request/response` pattern, say `HTTP/1.x`,
    as the most typical example, is to pend subsequent out-bound transportation through
    the underlying transport, wait until the expected response has been sent back from
    peer endpoint, as in-bound transportation through the underlying transport, before
    the next request is let-go to start off.

    The result is that transports (tcp connections for HTTP), wasted too much time in
    RTT (round-trip time). That's why MANY (yet SMALL) js/css files must be packed into
    FEW (yet LARGE) ones for decent page load time, while the total traffic amount is
    almost the same.

    Newer protocols like `HTTP/2` and `QUIC` (a.k.a `HTTP/3`) are addressing various issues,
    especially including the above said one, but suffering from legacy burden for backward
    compatibility with `HTTP/1.1`, they have gone not far.

    Building new applications with `Hosting Based Interface` - HBI, the classic
    `request/response` pattern can go naturally & very efficiently without imposing the
    dreaded RTT (if done correctly, see Caveats).


## Mechanism

    An `HBI` communication channel (wire) works Peer-to-Peer, each peer has 2 endpoints,
    the `posting` endpoint and the `hosting` endpoint.

    At any time, either peer can initiate a `posting conversation` for active communication.

    A `posting conversation` has 2 stages, -- the `posting stage` and the `after-posting stage`.
    During the `posting stage`, *peer scripting code*, i.e. textual code meant to be `landed`
    by peer's `hosting environment` (more on this later), optionally with binary data/stream,
    are sent through the underlying transport/wire.

    As the peer sees incoming traffic about the conversation, it establishes a `hosting-conversation`
    to accommodate the `landing` of *peer scripting code* received.

    A `hosting conversation` has just 1 stage, naming is not necessary but just call it
    `hosting stage`. During the `hosting stage`, the hosting peer uses its `hosting environment`
    to `land` whatever textual code sent by the posting peer. 

    `landing` is simply the execution of textual scripting code, with the chosen programming
    language / runtime, with the `hosting envrionment` as context. e.g. `exec()` is used with
    Python, and [Anko interpreter](https://github.com/mattn/anko "Anko's Github Home") is used
    with Golang.

    The `hosting environment` of the hosting peer is openly accessible by the *peer scripting code*
    from the posting peer, to the extent the hosting peer is willing of exposure.

    The *peer scripting code* just executes as being `landed`, it scripts the hosting peer
    for desired behavior.
    Another responsibility of the *peer scripting code* is that: if binary data/stream follows,
    it needs to receive or streamline the data properly, and with such responsibilities, it is
    called `receiving-code`.

    There normally occur subsequences as the hosting peer is being scripted to do anything,
    e.g. in case the posting peer is a software agent in behalf of its user to start a live
    video casting, all the user's subscribers should be notified of the starting of video 
    stream, and a streaming channel should be established to each ready subscriber, then the
    broadcaster should be notified how many subscribers will be watching.

    The *peer scripting code* instructs about all those things as `WHAT` to do, and the
    `hosting envirnoment` should expose enough artifacts implementing `HOW` to do each of those.

    Theoretically every artifact exposed by the hosting environment is a `function`, which takes
    specific number/type of arguments, generates side-effects, and returns specific number/type
    of result(s). While with Object-Oriented programming paradigm, there arose some types of
    `function`s carrying special semantics:
        * `constructor` function:
            that creates a new (tho not strictly necessary) object on each call
                *Note*: in HBI paradigm the `new` keyword should not appear for invocation of
                a `ctor` function. HBI peer script follows Python syntax and is different
                from Go/C++/Java/JavaScript syntax.
        * `reactor method` function:
            that has a `reactor` object bound to it

    The implementation of a `function` exposed by a `hosting environment`, normally does
    leverage the `hosting conversation` to send another set of *peer scripting code*, optionally
    with binary data/stream, back to the posting peer, for the subsequences be realized at the
    posting site. Additionally, the implementation can schedule more activities to happen later,
    and any activity can then start new `posting conversation` to the OP, i.e. communication
    in the reverse direction.

    Orchestration forms when multiple service/consumer nodes keep communicating p2p.


### Over vanilla TCP

    Python 3.7+
    ```python

    ```

    Go1
    ```go

    ```

    ES6
    ```js
// Coming later, not sooner ...
    ```


### Over `QUIC`

    Concurrent conversations can work upon QUIC streams, coming sooner than later ...

## Caveats

### For Overall Throughput

- Do NO `recv` at best, be `landing` peer scripts instead,
- Decided to do `recv`, ONLY do with a hosting conversation,
- Decided to `recv` with a posting conversation, ONLY do during the `after-posting` stage.

  though you are not technically prevented to `recv` during the `posting` stage,
  doing so will pend the underlying wire, stop pipeling of dataflow, thus harm
  a lot to overall throughput.
