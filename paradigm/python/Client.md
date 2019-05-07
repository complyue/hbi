# Hosting Based Interface

## Client Paradigm:

```python

import asyncio, hbi

from some.job.source import gather_jobs

POOR_THROUGHPUT = False

def find_service(locator):
    ...
    return {'host': '127.0.0.1', 'port': 3232}

jobs_queue, all_jobs_done = None, None

def has_more_jobs() -> bool:
    return not (
        jobs_queue is None or (
            jobs_queue.empty() and all_jobs_done.is_set()
        )
    )

po2peer: hbi.PostingEnd = None
ho4peer: hbi.HostingEnd = None

# get called when the hbi connection is made
async def __hbi_init__(po, ho):
    global po2peer: hbi.PostingEnd
    global ho4peer: hbi.HostingEnd

    po2peer, ho4peer = po, ho

# it's best for throughput to use such an asynchronous callback
# to react to results of service calls.
async def job_done(job_result):
    assert po2peer is not None and ho4peer is not None
    print('Job done:', job_result)

async def work_out(reconnect_wait=10):
    global jobs_queue, all_jobs_done
    # create the job queue and done event within a coroutine, for them to have
    # the correct loop associated as from the os thread running it
    jobs_queue = asyncio.Queue(100)
    all_jobs_done = asyncio.Event()
    # spawn a new concurrent green thread to gather jobs
    asyncio.create_task(gather_jobs(jobs_queue, all_jobs_done))

    service_addr = find_service(...)
    react_context = globals()

    job = None
    while job is not None or has_more_jobs():
        hbic = hbi.HBIC(service_addr, react_context)  # define the service connection
        try:
            # connect the service, get the posting/hosting endpoints
            async with hbic as (po, ho):  # auto close hbic as a context manager
                while job is not None or has_more_jobs():
                    if job is None:
                        job = await jobs_queue.get()
                    async with po.co() as co:  # establish a service conversation
                        # call service method `do_job()`
                        await co.send_code(rf'''
do_job({job.action!r}, {job.data_len!r})
''')
                        # the service method expects blob input,
                        # hbi wire can send binary streams very efficiently,
                        # a single binary buffer or an iterator of binary buffers can be passed to
                        # `co.send_data()` so long as the peer endpoint has the meta info beforehand,
                        # to infer correct data structure of summed bytes count.
                        await co.send_data(job.data)

                        # !! try best to avoid such synchronous service calls !!
                        if POOR_THROUGHPUT:
                            job_result = await co.recv_obj()
                            # !! this holds down throughput REALLY !!

                        job = None  # this job done
        except Exception:
            logger.error("Failed job processing over connection {hbic.net_ident!s}, retrying ...", exc_info=True)
            await asyncio.sleep(reconnect_wait)

asyncio.run(work_out())

```
