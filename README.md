# WebFS

A FUSE-based filesystem that mounts web resources (HTTP URLs, S3, etc.) declared by minimal configs as local files.

## Requirements
Linux, Mac(untested), or FreeBSD(untested) operating system with appropriate os-specific FUSE driver installed and available on $PATH:
### Linux
fusermount3 is available from all known distro package managers, ex. [Arch Linux](https://man.archlinux.org/man/fusermount3.1.en) (btw): `sudo pacman -S fuse3`
### MacOS (untested)
[macfuse](https://github.com/macfuse/macfuse) (formerly osxfuse) is recommended to be installed from their official [releases](https://github.com/macfuse/macfuse/releases/latest)
### FreeBSD (untested)
Appears [built-in](https://man.freebsd.org/cgi/man.cgi?fusefs)??

## Usage

```bash
go run ./cmd/main.go -nodes examples/pub_sources.json <mountpoint>
```
See `go run ./cmd/main.go --help` for full cli args.

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
- [ ] Read-only MVP unit tests 

### TODO
- [ ] Test MacOS
- [ ] Test FreeBSD
- [ ] Socket API
- [ ] "Magic File" add i.e. `echo "https://example.com/file.txt" > /webfs/file.txt.wfs`
- [ ] Additional Adapters
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
- [ ] Rename
  - FTW (FUSE the Web), AnyFS, WebFuse, WebFusion, WFS, AnyFuse, AnyFusion, Net{Fuse,FS,...},
