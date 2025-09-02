# W[eb]FS

A FUSE (Filesystem in User SpacE) implementation that serves web resources (Public HTTP URLs, S3, etc.) as local files declared by minimal configs. While similar to and implementing the same protocol as the excellent [rclone](https://github.com/rclone/rclone) and proprietary cloud drive clients i.e. Google Drive & Dropbox, they are geared towards mounting/mirroring entire cloud drives locally with the cloud service centrally managing shared file names, directory trees, etc. The focus for WFS is on serving "links as files" so the entire internet can become your cloud provider, named and organized however you want just like any other on-device files.

Currently it supports HTTP(S) URL sources and a subset of system file operations (list + attributes, open, read ).

**NOTE:** WFS is in early, active development in my spare time, with a stable API still settling. If just curious and tinkering go for it expecting breaking changes, and *only* ever add reputable file sources you would physically download to your machine anyways. It should not be even considered to be uttered outloud in the same context of anything resembling a production or secured environment 😄  

## Requirements

Linux, Mac(untested), or FreeBSD(untested) operating system with appropriate os-specific FUSE binary installed and available on $PATH:

### Linux

fusermount3 is available from all known major distro package managers, ex. [Arch Linux](https://man.archlinux.org/man/fusermount3.1.en) (btw): `sudo pacman -S fuse3`

### MacOS (untested)

[macfuse](https://github.com/macfuse/macfuse) (formerly osxfuse) is recommended to be installed from their official [releases](https://github.com/macfuse/macfuse/releases/latest)

### FreeBSD (untested)

Appears [built-in](https://man.freebsd.org/cgi/man.cgi?fusefs)??

## Usage

```bash
go run ./cmd/main.go --nodes examples/pub_sources.json <mountpoint>

# In another terminal (for now):
ls <mountpoint>
# Use like any other local file
<mpv|vlc|some-video-player> <mountpoint>/bbb/BigBuckBunny.mp4
```

See `go run ./cmd/main.go --help` for full cli args.

If something goes terribly wrong e.g. processed killed without signals and a chance to gracefully exit, try `fusermount -u <mountpoint>` to unmount the filesystem.

## Build

```bash
go build -o bin/webfs ./cmd/main.go
```

## Status

### DONE

- [x] Python POC
- [x] [Migrate & Iterate Python POC to Go MVP](https://github.com/BrettBedarf/webfs/pull/1)
  - Supports minimal list, open, attributes sys calls for public http/https urls  
- [x] e2e testing pattern & basic MVP e2e tests

### IN PROGRESS

- [ ] Docs
- [ ] Unit tests for initial read-only fs

### TODO

- [ ] YouTube adapter & demo
- [ ] Test MacOS
- [ ] Test FreeBSD
- [ ] "Magic File" add i.e. `echo "https://example.com/file.txt" > /webfs/file.txt.wfs`
- [ ] Hooks integration point (stdio "on-open", "on-update", "on-fail", "on-fetch", etc)
- [ ] Socket API
- [ ] Additional Protocol Providers
  - [ ] S3
  - [ ] FTP
  - [ ] SFTP
  - [ ] Google Drive
  - [ ] Dropbox
  - [ ] OneDrive
  - [ ] GCS
  - [ ] Azure Blob Storage
  - [ ] IPFS
- [ ] Lua? Adapters extension point
