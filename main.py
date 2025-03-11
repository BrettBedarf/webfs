#!/usr/bin/env python3
import errno
import json
import logging
import os
import socket
import stat
import sys
import threading
import time
import pyfuse3
from pyfuse3 import EntryAttributes, RequestContext, FileHandleT, FileNameT, Operations
import trio
import requests
import cachetools
import asyncio
import aiohttp
import functools
from datetime import datetime
from typing import cast
import debugpy

# This should not block execution
debugpy.listen(("0.0.0.0", 5678))
print("Debugpy is listening on port 5678; wait_for_client")
# debugpy.wait_for_client()

source_json = json.load(open("tests/fixtures/pub_sources.json"))


# Global mapping of local filenames to HTTP URLs.
source_files = {}
for source_file in source_json["categories"][0]["videos"]:
    source_url: str = source_file["sources"][0]
    filename = source_url.split("/")[-1]
    print(f"{filename} : {source_url}")

    source_files[filename] = source_url

FILES_LOCK = threading.Lock()
# TODO: INODE_MAP should be a mapping of inode -> file
INODE_MAP = {}  # filename -> inode
NEXT_INODE = 2  # starting inode (root is 1)

MAX_FH = (
    2**31 - 1
)  # 32-bits ensures compat with libfuse and more than enough open handles

# Read cache configuration
DEFAULT_CHUNK_SIZE = 2 * 1024 * 1024  # 4MB per chunk
CACHE_MAX_SIZE = 200 * 1024 * 1024  # 100MB total
NUM_CACHE_CHUNKS = CACHE_MAX_SIZE // DEFAULT_CHUNK_SIZE
MAX_PREFETCH_AHEAD = 100 * 1024 * 1024  # e.g., 100MB ahead
# How many chunks to fetch concurrently each batch.
PREFETCH_BATCH_SIZE = 6

logger = logging.getLogger()
logger.setLevel(logging.DEBUG)

console_handler = logging.StreamHandler()
console_handler.setLevel(logging.DEBUG)
file_handler = logging.FileHandler(
    f"logs/httpfs_{datetime.now().strftime('%Y%m%d_%H%M%S')}.log"
)
file_handler.setLevel(logging.DEBUG)

formatter = logging.Formatter("%(asctime)s [%(levelname)s] %(message)s")
console_handler.setFormatter(formatter)
file_handler.setFormatter(formatter)

logger.addHandler(console_handler)
logger.addHandler(file_handler)


# Decorator to log elapsed time for each function/method.
def log_time(func):
    @functools.wraps(func)
    def wrapper(*args, **kwargs):
        start = time.perf_counter()
        result = func(*args, **kwargs)
        elapsed = time.perf_counter() - start
        logger.debug(f"{func.__name__} took {elapsed:.4f} seconds")
        return result

    return wrapper


file_attributes_cache = {}


@log_time
def get_file_attr(filename: str) -> EntryAttributes:
    logger.debug("Getting file attributes for '%s'", filename)
    if filename in file_attributes_cache:
        logger.debug("Returning cached file attributes for '%s'", filename)
        return file_attributes_cache[filename]

    inode = INODE_MAP.get(filename)
    url = source_files.get(filename)
    if inode is None or url is None:
        logger.error("File not found: '%s'", filename)
        raise FileNotFoundError

    try:
        logger.info("Fetching HEAD from remote")
        r = requests.head(url, allow_redirects=True)
        size = int(r.headers.get("Content-Length", 0))
        logger.debug("Size of '%s': %d bytes", filename, size)
    except Exception as e:
        logger.error("Error fetching HEAD for '%s': %s", filename, e)
        size = 0

    now_ns = int(time.time() * 1e9)
    attr = pyfuse3.EntryAttributes()
    attr.st_ino = inode
    # Use cast(...) to satisfy the type checker for st_mode.
    attr.st_mode = cast(pyfuse3.ModeT, stat.S_IFREG | 0o444)
    attr.st_size = size
    attr.st_uid = os.getuid()
    attr.st_gid = os.getgid()
    attr.st_atime_ns = now_ns
    attr.st_mtime_ns = now_ns
    attr.st_ctime_ns = now_ns
    attr.st_nlink = 1

    file_attributes_cache[filename] = attr
    return attr


@log_time
def get_root_attr() -> EntryAttributes:
    logger.debug("Getting root attributes")
    now_ns = int(time.time() * 1e9)
    attr = pyfuse3.EntryAttributes()
    attr.st_ino = pyfuse3.ROOT_INODE
    # Mark directory mode with 755 perms
    attr.st_mode = cast(pyfuse3.ModeT, stat.S_IFDIR | 0o755)
    attr.st_uid = os.getuid()
    attr.st_gid = os.getgid()
    attr.st_size = 0
    attr.st_atime_ns = now_ns
    attr.st_mtime_ns = now_ns
    attr.st_ctime_ns = now_ns
    attr.st_nlink = 2
    return attr


# LRU cache to store chunks
file_chunk_cache = cachetools.LRUCache(maxsize=NUM_CACHE_CHUNKS)
cache_lock = threading.Lock()

# Global dict for per-chunk events
ongoing_requests = {}
ongoing_lock = threading.Lock()

# Global dict to track active prefetch threads per URL
prefetch_threads = {}
prefetch_lock = threading.Lock()


# Global mapping: file URL -> persistent requests.Session
sessions = {}
session_lock = threading.Lock()
IDLE_TIMEOUT = 300  # seconds


@log_time
def get_session_for_url(url):
    now = time.time()
    with session_lock:
        if url in sessions:
            session, _ = sessions[url]
            sessions[url] = (session, now)
            return session
        else:
            s = requests.Session()
            sessions[url] = (s, now)
            return s


@log_time
def cleanup_sessions():
    while True:
        time.sleep(60)  # check every minute
        now = time.time()
        with session_lock:
            for url in list(sessions.keys()):
                session, last_used = sessions[url]
                if now - last_used > IDLE_TIMEOUT:
                    session.close()
                    del sessions[url]
                    # also clear the redirect cache
                    del redirect_cache[url]


# Start cleanup thread
threading.Thread(target=cleanup_sessions, daemon=True).start()

# Global cache for resolved URLs
redirect_cache = {}
redirect_cache_lock = threading.Lock()


@log_time
def resolve_redirect(url):
    with redirect_cache_lock:
        if url in redirect_cache:
            return redirect_cache[url]
    session = get_session_for_url(url)
    r = session.head(url, allow_redirects=True)
    final_url = r.url
    with redirect_cache_lock:
        redirect_cache[url] = final_url
    return final_url


@log_time
def maybe_prefetch(url, current_read_offset, total_size):
    # Determine how far we've cached for this URL.
    with cache_lock:
        cached_offsets = [off for (u, off) in file_chunk_cache if u == url]
    highest_cached = max(cached_offsets) if cached_offsets else current_read_offset
    target = current_read_offset + MAX_PREFETCH_AHEAD
    if highest_cached < target:
        # Spawn a thread to prefetch continuously from the current highest offset up to the target.
        threading.Thread(
            target=prefetch,
            args=(
                url,
                highest_cached,
                DEFAULT_CHUNK_SIZE,
                target - highest_cached,
                total_size,
            ),
            daemon=True,
        ).start()


async def fetch_chunk(session, url, offset, chunk_size):
    headers = {"Range": f"bytes={offset}-{offset + chunk_size - 1}"}
    start = time.perf_counter()
    async with session.get(url, headers=headers) as response:
        if response.status in (200, 206):
            ret = await response.read()
            end = time.perf_counter() - start
            logger.debug(
                f"fetch_chunk: offset={offset}, chunk_size={chunk_size}, elapsed={end:.4f} seconds"
            )
            return ret
        else:
            end = time.perf_counter() - start
            logger.debug(
                f"fetch_chunk FAIL: offset={offset}, chunk_size={chunk_size}, elapsed={end:.4f} seconds"
            )
            raise Exception(f"HTTP error {response.status}")


@log_time
# Async function to fetch multiple chunks concurrently.
async def fetch_chunks_async(url, offsets, chunk_size):
    async with aiohttp.ClientSession() as session:
        tasks = [fetch_chunk(session, url, offset, chunk_size) for offset in offsets]
        return await asyncio.gather(*tasks)


@log_time
# Synchronous wrapper around the async function for fuse compat
def fetch_chunks_sync(url, offsets, chunk_size, total_size):
    # Only fetch offsets less than the file's total size.
    valid_offsets = [offset for offset in offsets if offset < total_size]
    if not valid_offsets:
        return []
    result = asyncio.run(fetch_chunks_async(url, valid_offsets, chunk_size))
    # Log the current cache size in MB and active prefetch threads.
    with cache_lock:
        total_bytes = sum(len(chunk) for chunk in file_chunk_cache.values())
    cache_mb = total_bytes / (1024 * 1024)
    with prefetch_lock:
        active_prefetch = len(prefetch_threads)
    logging.debug(
        f"fetch_chunks_sync: current file_chunk_cache size: {cache_mb:.2f} MB; active prefetch threads: {active_prefetch}"
    )
    return result


@log_time
def get_file_chunk(file_url, chunk_start, chunk_size, total_size):
    cache_key = (file_url, chunk_start)
    with cache_lock:
        if cache_key in file_chunk_cache:
            return file_chunk_cache[cache_key]

    is_ongoing = False
    with ongoing_lock:
        event = ongoing_requests.get(cache_key)
        if event:
            is_ongoing = True
        else:
            event = threading.Event()
            ongoing_requests[cache_key] = event
    if is_ongoing:
        # Wait for the fetching thread to complete
        event.wait()
        with cache_lock:
            return file_chunk_cache.get(cache_key, b"")
    else:
        chunk = fetch_chunks_sync(file_url, chunk_start, chunk_size, total_size)
        with cache_lock:
            file_chunk_cache[cache_key] = chunk
        with ongoing_lock:
            ongoing_requests[cache_key].set()  # important to signal waiting threads
            del ongoing_requests[cache_key]
        return chunk


@log_time
def prefetch(url, start_offset, chunk_size, max_prefetch_bytes, total_size):
    """
    Prefetch multiple chunks at once until we fill up max_prefetch_bytes.
    """
    current = start_offset
    end_offset = start_offset + max_prefetch_bytes

    while current < end_offset:
        # Accumulate a list of offsets we still need (and aren't cached yet).
        offsets_to_fetch = []
        for _ in range(PREFETCH_BATCH_SIZE):
            if current >= end_offset:
                break
            cache_key = (url, current)
            with cache_lock:
                # If it's already cached, skip it
                if cache_key in file_chunk_cache:
                    current += chunk_size
                    continue
            offsets_to_fetch.append(current)
            current += chunk_size

        # If we didn't find any offsets to fetch this round, break out
        if not offsets_to_fetch:
            break

        # Now fetch them in one async batch
        chunks = fetch_chunks_sync(url, offsets_to_fetch, chunk_size, total_size)
        # Store them in the cache
        with cache_lock:
            for i, offset in enumerate(offsets_to_fetch):
                file_chunk_cache[(url, offset)] = chunks[i]

    # Remove thread marker when done
    with prefetch_lock:
        prefetch_threads.pop(url, None)


_next_fh = 1
_fh_lock = threading.Lock()
# Mapping: file handle -> metadata dictionary (e.g. inode and allocation timestamp)
open_handles = {}


def get_next_fh(inode):
    global _next_fh
    with _fh_lock:
        candidate = _next_fh
        _next_fh = (_next_fh + 1) % MAX_FH
        while candidate in open_handles:
            candidate = _next_fh
            _next_fh = (_next_fh + 1) % MAX_FH
        open_handles[candidate] = {"inode": inode, "allocated_at": time.time()}
        return FileHandleT(candidate)


class HTTPFS(Operations):
    async def lookup(self, parent_inode, name, ctx):
        if parent_inode != pyfuse3.ROOT_INODE:
            logger.error("lookup: non-root parent inode %d", parent_inode)
            raise pyfuse3.FUSEError(errno.ENOENT)
        filename = name.decode("utf-8") if isinstance(name, bytes) else name
        # if filename[0] != ".":
        #     logger.debug("lookup: parent_inode=%d, name=%s", parent_inode, name)
        with FILES_LOCK:
            if filename not in source_files:
                logging.error("lookup: '%s' not found", filename)
                raise pyfuse3.FUSEError(errno.ENOENT)
            inode = INODE_MAP.get(filename)
            if inode is None:
                global NEXT_INODE
                inode = NEXT_INODE
                INODE_MAP[filename] = inode
                NEXT_INODE += 1
                logger.debug("lookup: assigned new inode %d to '%s'", inode, filename)
        return get_file_attr(filename)

    async def getattr(self, inode, ctx):
        logger.debug("getattr: inode=%d", inode)
        if inode == pyfuse3.ROOT_INODE:
            return get_root_attr()
        with FILES_LOCK:
            filename = next((fn for fn, ino in INODE_MAP.items() if ino == inode), None)
        if filename is None:
            logger.error("getattr: inode %d not found", inode)
            raise pyfuse3.FUSEError(errno.ENOENT)
        return get_file_attr(filename)

    async def opendir(self, inode: int, ctx: RequestContext) -> FileHandleT:
        logger.debug("opendir: inode=%d", inode)
        if inode != pyfuse3.ROOT_INODE:
            logger.error("opendir: inode %d is not root", inode)
            raise pyfuse3.FUSEError(errno.ENOTDIR)
        fh = get_next_fh(inode)
        return fh

    async def readdir(
        self, fh: FileHandleT, start_id: int, token: pyfuse3.ReaddirToken
    ) -> None:
        logger.debug("readdir: fh=%d, start_id=%d", fh, start_id)
        # if fh != pyfuse3.ROOT_INODE:
        #     logger.error("readdir: file handle %d is not root", fh)
        #     return

        # Build directory entry list including '.' and '..'
        entries = [
            (FileNameT(b"."), get_root_attr()),
            (FileNameT(b".."), get_root_attr()),
        ]
        with FILES_LOCK:
            for filename in source_files.keys():
                try:
                    attr = get_file_attr(filename)
                    entries.append((FileNameT(filename.encode("utf-8")), attr))
                    logger.debug("readdir: adding entry '%s'", filename)
                except FileNotFoundError:
                    logger.error("readdir: file '%s' not found", filename)
                    continue

        # Use the pyfuse3.readdir_reply method to add each entry.
        for idx, (name, attr) in enumerate(entries):
            if idx < start_id:
                continue
            next_id = idx + 1
            if not pyfuse3.readdir_reply(token, name, attr, next_id):
                break

    async def open(
        self, inode: int, flags: int, ctx: RequestContext
    ) -> pyfuse3.FileInfo:
        logger.debug("open: inode=%d, flags=%d", inode, flags)
        if flags & (os.O_WRONLY | os.O_RDWR):
            logger.error("open: write access denied for inode=%d", inode)
            raise pyfuse3.FUSEError(errno.EACCES)

        fi = pyfuse3.FileInfo(
            fh=get_next_fh(inode),
            direct_io=True,  # take over responsibilty for caching, buffering, etc from kernel
        )

        return fi

    async def read(self, fh: FileHandleT, off: int, size: int) -> bytes:
        logger.debug("read: fh=%d, off=%d, size=%d", fh, off, size)
        with _fh_lock:
            handle_details = open_handles.get(fh)
            ino = handle_details.get("inode") if handle_details else None
            if not ino:
                logger.error(f"no inode found for handle {fh}")
                raise pyfuse3.FUSEError(errno.ENOENT)
        with FILES_LOCK:
            # TODO: refactor INODE_MAP to be inode -> filename
            filename = next((fn for fn, _ino in INODE_MAP.items() if _ino == fh), None)
            if filename is None:
                raise pyfuse3.FUSEError(errno.ENOENT)
            url = source_files.get(filename)
        if not url:
            raise pyfuse3.FUSEError(errno.ENOENT)
        attr = get_file_attr(filename)
        total_size = attr.st_size
        # Determine chunk boundaries covering the requested range.
        start_offset = off - (off % DEFAULT_CHUNK_SIZE)
        end_offset = off + size
        # Offsets at which each chunk starts
        offsets = list(range(start_offset, end_offset, DEFAULT_CHUNK_SIZE))
        # If range() is empty, force at least one offset
        if not offsets:
            offsets = [start_offset]

        # Fetch all needed chunks concurrently.
        # fetch_chunks_sync expects a list of offsets.
        chunks = fetch_chunks_sync(url, offsets, DEFAULT_CHUNK_SIZE, total_size)

        # Assemble the requested data:
        data = bytearray()
        for i, chunk in enumerate(chunks):
            # For the first chunk, start at the proper offset.
            if i == 0:
                start_in_chunk = off - start_offset
                data.extend(chunk[start_in_chunk:])
            else:
                data.extend(chunk)
        # Trim data to exactly 'size' bytes.
        result = bytes(data[:size])

        # Trigger prefetch for data beyond what was just read.
        maybe_prefetch(url, off + size, total_size)

        logger.debug("read: returning %d bytes", len(result))
        return result


def listen_for_updates(port=9000):
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("localhost", port))
    sock.listen(5)
    logger.info("Update server listening on port %d", port)
    while True:
        conn, _ = sock.accept()
        logger.debug("Update server: connection accepted")
        data = conn.recv(1024).decode("utf-8")
        logger.debug("Update server: received data: %s", data)
        try:
            update = json.loads(data)
            filename = update.get("filename")
            url = update.get("url")
            if filename and url:
                with FILES_LOCK:
                    source_files[filename] = url
                    if filename not in INODE_MAP:
                        global NEXT_INODE
                        INODE_MAP[filename] = NEXT_INODE
                        NEXT_INODE += 1
                logger.info("Added mapping: '%s' -> '%s'", filename, url)
                conn.send(b"OK")
            else:
                logger.error("Update server: invalid data received")
                conn.send(b"ERROR: Invalid data")
        except Exception as e:
            logger.exception("Update server: error processing update")
            conn.send(f"ERROR: {str(e)}".encode("utf-8"))
        conn.close()
        logger.debug("Update server: connection closed")


async def main(mountpoint):
    fs = HTTPFS()
    fuse_opts = set(pyfuse3.default_options)
    # fuse_opts.add(
    #     "allow_other"
    # )  # Allow all users (not just the mounter) to access the FS.
    fuse_opts.add("ro")  # Mount as read-only.
    fuse_opts.add("fsname=httpls")  # Set a custom filesystem name ("httpls").
    # fuse_opts.add("max_read=65536")  # Limit each read request to 64KB.
    fuse_opts.add(
        "auto_unmount"
    )  # Automatically unmount the FS if the process terminates.
    fuse_opts.add(
        "nodiratime"
    )  # Do not update directory access times (reduces overhead).

    pyfuse3.init(fs, mountpoint, fuse_opts)
    logger.info("FUSE filesystem mounted on '%s'", mountpoint)
    try:
        await pyfuse3.main()
    finally:
        logger.info("Unmounting filesystem")
        pyfuse3.close()


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <mountpoint>")
        sys.exit(1)
    mountpoint = sys.argv[1]

    # Pre-populate the inode map.
    with FILES_LOCK:
        for filename in list(source_files.keys()):
            if filename not in INODE_MAP:
                INODE_MAP[filename] = NEXT_INODE
                NEXT_INODE += 1

    threading.Thread(target=listen_for_updates, daemon=True).start()

    # Allow clean exit with Ctrl+C.
    def sigint_handler(signum, frame):
        logger.info("SIGINT received, exiting...")
        pyfuse3.close()
        sys.exit(0)

    # signal.signal(signal.SIGINT, sigint_handler)

    trio.run(main, mountpoint)
